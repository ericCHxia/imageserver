package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"imageserver/config"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func GetImageHandle(p Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := c.MustGet("request").(*ImageRequest)
		file := request.FilePath

		obj, err := p.GetObject(file)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		img, err := vips.NewImageFromBuffer(obj.Body)
		if err != nil {
			logrus.Errorln("Error reading image:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading image"})
			return
		}
		defer img.Close()

		if img.Format() == vips.ImageTypeUnknown {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid Image"})
			return
		}
		logrus.Infof("Image option: %+v\n", request.Options)
		imageBytes, err := Process(img, request.Options)
		if err != nil {
			logrus.Errorln("Error processing image:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error processing image"})
			return
		}

		contentType := "image/" + ImageTypes[img.Format()]
		if request.Options.Type != vips.ImageTypeUnknown {
			contentType = "image/" + ImageTypes[request.Options.Type]
		}
		c.Writer.Header().Set("Content-Type", contentType)
		c.Writer.Header().Set("ETag", obj.ETag)
		c.Writer.Header().Set("Last-Modified", obj.LastModified.Format(http.TimeFormat))
		c.Writer.WriteHeader(http.StatusOK)
		_, err = c.Writer.Write(imageBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error writing response"})
			return
		}
	}
}

func CacheControlHandle(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() < 300 {
			c.Writer.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cfg.CacheController.MaxAge))
		}
	}
}

func CacheRequestHandle(ctx context.Context, marshal *marshaler.Marshaler) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := c.MustGet("request").(*ImageRequest)

		hash := sha256.New()
		err := gob.NewEncoder(hash).Encode(request)
		if err != nil {
			logrus.Errorln("Error encoding request", err)
			c.Next()
			return
		}
		key := hash.Sum(nil)

		var response = new(responseCache)
		if _, err := marshal.Get(ctx, key, response); err == nil {
			c.Writer.WriteHeader(response.Status)
			for k, values := range response.Header {
				for _, v := range values {
					c.Writer.Header().Set(k, v)
				}
			}
			_, err := c.Writer.Write(response.Data)
			if err != nil {
				logrus.Errorln("Error writing cached response", err)
			} else {
				c.Abort()
				return
			}
		}

		c.Writer = &cachedWriter{ResponseWriter: c.Writer, store: marshal, context: ctx, key: key}
		c.Next()

		if c.Writer.Status() >= 300 {
			err := marshal.Delete(ctx, key)
			if err != nil {
				logrus.Errorln("Error deleting cache", err)
			}
		}
	}
}

func VerifyHMACHandle(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Hmac.Activated {
			c.Next()
			return
		}

		message := c.GetString("message")
		messageMAC := c.GetString("hmac")
		if !hmacVerify(cfg.Hmac, message, messageMAC) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func ParsePathHandle(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		paths := strings.Split(path, "/")
		var options []string
		var messageMAC string
		var filepath string
		var message string
		if cfg.Hmac.Activated {
			if len(paths) < 4 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
				c.Abort()
				return
			}
			messageMAC = paths[1]
			options = paths[2 : len(paths)-1]
			filepath = paths[len(paths)-1]
			message = "/" + strings.Join(paths[2:], "/")
		} else {
			if len(paths) < 3 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
				c.Abort()
				return
			}
			options = paths[1 : len(paths)-1]
			filepath = paths[len(paths)-1]
		}

		if cfg.Base64Path {
			if decoded, err := base64.RawURLEncoding.DecodeString(filepath); err == nil {
				filepath = string(decoded)
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "Invalid Path"})
				c.Abort()
				return
			}
		} else {
			filepath, _ = url.PathUnescape(filepath)
		}
		c.Set("filepath", filepath)
		c.Set("options", options)
		c.Set("hmac", messageMAC)
		c.Set("message", message)
		c.Next()
	}
}

func ParseRequestHandle() gin.HandlerFunc {
	return func(c *gin.Context) {
		options := c.GetStringSlice("options")
		file := c.GetString("filepath")
		request := newImageRequest()
		request.FilePath = file
		optionPattern := regexp.MustCompile(`(?P<action>[a-z]+):(?P<value>[a-zA-Z0-9]+)`)
		for _, option := range options {
			if !optionPattern.MatchString(option) {
				continue
			}
			matches := optionPattern.FindStringSubmatch(option)
			action := matches[1]
			value := matches[2]
			switch action {
			case "w":
				var width int
				_, err := fmt.Sscanf(value, "%d", &width)
				if err != nil {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid width %s", value)})
					return
				}
				if width > 0 {
					request.Width = width
				}
			case "f":
				for imageType, name := range ImageTypes {
					if name == value {
						request.Type = imageType
						break
					}
				}
				if request.Type == vips.ImageTypeUnknown {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid format"})
					return
				}
			}
		}
		c.Set("request", request)
		c.Next()
	}
}

func ModifiedHandle(p Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		filePath := c.GetString("filepath")
		key, err := p.GetObjectAttribute(filePath)
		if err != nil {
			c.Next()
			return
		}
		if key.ETag == c.GetHeader("If-None-Match") {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}
		if key.LastModified.Format(http.TimeFormat) == c.GetHeader("If-Modified-Since") {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}
	}
}
