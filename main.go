package main

import (
	"context"
	"github.com/allegro/bigcache/v3"
	s3config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	bigCacheStore "github.com/eko/gocache/store/bigcache/v4"
	"github.com/gin-gonic/gin"
	"github.com/ilyakaznacheev/cleanenv"
	"imageserver/config"
	"net/http"
	"time"
)

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

	provider := NewS3ClientWithObjectCache(s3.NewFromConfig(sdkConfig), marshal, &cfg)

	r := gin.Default()
	r.UseRawPath = true
	r.UnescapePathValues = false
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	handles := make([]gin.HandlerFunc, 0)

	handles = append(handles, ParsePathHandle(cfg))
	if cfg.Hmac.Activated {
		handles = append(handles, VerifyHMACHandle(cfg))
	}

	handles = append(handles, ParseRequestHandle())
	if cfg.Cache.Activated {
		handles = append(handles, ModifiedHandle(provider), CacheRequestHandle(context.Background(), marshal))
	}

	handles = append(handles, GetImageHandle(provider), CacheControlHandle(cfg))

	r.NoRoute(handles...)
	err = r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
