package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"github.com/allegro/bigcache/v3"
	s3config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	bigCacheStore "github.com/eko/gocache/store/bigcache/v4"
	"github.com/gin-gonic/gin"
	"github.com/h2non/bimg"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/sirupsen/logrus"
	"imageserver/config"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func hmacVerify(cfg config.HMAC, message string, messageMAC string) bool {
	mac := hmac.New(sha256.New, []byte(cfg.SecretKey))
	mac.Write([]byte(message))
	mac.Write([]byte(cfg.Salt))
	expectedMAC := mac.Sum(nil)
	messageMACBytes, err := base64.RawURLEncoding.DecodeString(messageMAC)
	if err != nil {
		return false
	}
	return hmac.Equal(messageMACBytes, expectedMAC)
}

func GetImages(cfg config.Config, s3Client *S3ClientWithObjectCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := c.MustGet("request").(*ImageRequest)
		file := request.FilePath

		obj, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: &cfg.S3.Bucket,
			Key:    &file,
		})
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logrus.Errorln("Error closing body:", err)
			}
		}(obj.Body)

		imageBytes, err := io.ReadAll(obj.Body)
		logrus.Infoln("Image size:", len(imageBytes))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading file"})
			return
		}
		img := bimg.NewImage(imageBytes)

		if img.Type() == "unknown" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid Image"})
			return
		}
		fmt.Printf("Image option: %+v\n", request.Options)
		imageBytes, err = img.Process(request.Options)
		if err != nil {
			logrus.Errorln("Error processing image:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error processing image"})
			return
		}

		contentType := "image/" + img.Type()
		if request.Options.Type != bimg.UNKNOWN {
			contentType = "image/" + bimg.ImageTypeName(request.Options.Type)
		}
		c.Writer.Header().Set("Content-Type", contentType)
		if cfg.CacheController.Activated {
			c.Writer.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cfg.CacheController.MaxAge))
		}
		c.Writer.Header().Set("ETag", *obj.ETag)
		c.Writer.Header().Set("Last-Modified", obj.LastModified.Format(http.TimeFormat))
		c.Writer.WriteHeader(http.StatusOK)
		_, err = c.Writer.Write(imageBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error writing response"})
			return
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
		pathes := strings.Split(path, "/")
		var options []string
		var messageMAC string
		var filepath string
		var message string
		if cfg.Hmac.Activated {
			if len(pathes) < 4 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
				c.Abort()
				return
			}
			messageMAC = pathes[1]
			options = pathes[2 : len(pathes)-1]
			filepath = pathes[len(pathes)-1]
			message = "/" + strings.Join(pathes[2:], "/")
		} else {
			if len(pathes) < 3 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
				c.Abort()
				return
			}
			options = pathes[1 : len(pathes)-1]
			filepath = pathes[len(pathes)-1]
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
				for imageType, name := range bimg.ImageTypes {
					if name == value {
						request.Type = imageType
						break
					}
				}
				if request.Type == bimg.UNKNOWN {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid format"})
					return
				}
			}
		}
		c.Set("request", request)
		c.Next()
	}
}

func CacheRequestHandle(ctx context.Context, cfg config.Config, marshal *marshaler.Marshaler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Cache.Activated {
			c.Next()
			return
		}
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
			for k, vals := range response.Header {
				for _, v := range vals {
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

func ModifiedHandle(cfg config.Config, marshal *marshaler.Marshaler) gin.HandlerFunc {
	return func(c *gin.Context) {
		filePath := c.GetString("filepath")
		key := "attr_" + cfg.S3.Bucket + "/" + filePath
		attr := new(s3.GetObjectOutput)
		if _, err := marshal.Get(context.Background(), key, attr); err == nil {
			if attr.ETag != nil && *attr.ETag == c.GetHeader("If-None-Match") {
				c.Status(http.StatusNotModified)
				c.Abort()
				return
			}
			if attr.LastModified != nil && attr.LastModified.Format(http.TimeFormat) == c.GetHeader("If-Modified-Since") {
				c.Status(http.StatusNotModified)
				c.Abort()
				return
			}
		}
	}
}

func main() {
	var cfg config.Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		panic(err)
	}

	sdkConfig, err := s3config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()
	cacheClient, _ := bigcache.New(ctx, bigcache.DefaultConfig(time.Duration(cfg.Cache.Time)*time.Second))
	cacheStore := bigCacheStore.NewBigcache(cacheClient)
	marshal := marshaler.New(cache.New[any](cacheStore))

	s3Client := NewS3ClientWithObjectCache(s3.NewFromConfig(sdkConfig), marshal, &cfg)

	r := gin.Default()
	r.UseRawPath = true
	r.UnescapePathValues = false
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.NoRoute(ParsePathHandle(cfg), VerifyHMACHandle(cfg), ModifiedHandle(cfg, marshal), ParseRequestHandle(), CacheRequestHandle(context.Background(), cfg, marshal), GetImages(cfg, s3Client))
	err = r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
