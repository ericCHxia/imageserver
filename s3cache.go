package main

import (
	"bytes"
	"context"
	"errors"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/sirupsen/logrus"
	"imageserver/config"
	"io"
	"time"
)

type S3ClientWithObjectCache struct {
	*s3.Client
	cacheContext context.Context
	cache        *marshaler.Marshaler
	cfg          *config.Config
}

type S2ObjectAttributeCache struct {
	ETag         string
	LastModified *time.Time
}

type S3ObjectCache struct {
	S2ObjectAttributeCache
	Body []byte
}

func NewS3ObjectCache(object *s3.GetObjectOutput) (*S3ObjectCache, error) {
	data, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, err
	}
	object.Body.Close()
	object.Body = io.NopCloser(bytes.NewReader(data))
	return &S3ObjectCache{
		S2ObjectAttributeCache: S2ObjectAttributeCache{
			ETag:         *object.ETag,
			LastModified: object.LastModified,
		},
		Body: data,
	}, nil
}

func (s *S3ObjectCache) ToGetObjectOutput() *s3.GetObjectOutput {
	return &s3.GetObjectOutput{
		Body:         io.NopCloser(bytes.NewReader(s.Body)),
		ETag:         &s.ETag,
		LastModified: s.LastModified,
	}
}

func (s *S3ClientWithObjectCache) setFlesh(key string) {
	err := s.cache.Set(s.cacheContext, "flesh_"+key, nil, store.WithExpiration(time.Duration(s.cfg.S3.FleshTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 flesh cache set error", err)
	}
}

func (s *S3ClientWithObjectCache) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if !s.cfg.Cache.Activated {
		return s.Client.GetObject(ctx, params, optFns...)
	}
	key := *params.Bucket + "/" + *params.Key

	data := &S3ObjectCache{}
	_, err := s.cache.Get(s.cacheContext, key, &data)
	if err == nil {
		logrus.Infoln("S3 cache hit", key)
		_, err = s.cache.Get(s.cacheContext, "flesh_"+key, nil)
		if err == nil {
			return data.ToGetObjectOutput(), nil
		}
		params.IfNoneMatch = &data.ETag
		params.IfModifiedSince = data.LastModified
	}

	res, err := s.Client.GetObject(ctx, params, optFns...)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) && re.ResponseError.HTTPStatusCode() == 304 {
			logrus.Infoln("S3 not modified", key)
			err = nil
			res = data.ToGetObjectOutput()
			s.setFlesh(key)
		}
		return res, err
	}

	data, err = NewS3ObjectCache(res)
	if err != nil {
		logrus.Errorln("S3 cache convert error", err)
		return res, nil
	}
	err = s.cache.Set(s.cacheContext, key, data, store.WithExpiration(time.Duration(s.cfg.S3.CacheTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 cache set error", err)
	}
	err = s.cache.Set(s.cacheContext, "attr_"+key, &data.S2ObjectAttributeCache, store.WithExpiration(time.Duration(s.cfg.S3.CacheTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 cache attr set error", err)
	}
	s.setFlesh(key)
	return res, nil
}

func NewS3ClientWithObjectCache(s3Client *s3.Client, cache *marshaler.Marshaler, cfg *config.Config) *S3ClientWithObjectCache {
	return &S3ClientWithObjectCache{
		Client:       s3Client,
		cacheContext: context.Background(),
		cache:        cache,
		cfg:          cfg,
	}
}
