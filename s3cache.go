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

type S3Client struct {
	Provider
	Client       *s3.Client
	cacheContext context.Context
	cache        *marshaler.Marshaler
	cfg          *config.Config
}

type S3ObjectAttributeCache struct {
	ETag         string
	LastModified time.Time
}

type S3ObjectCache struct {
	S3ObjectAttributeCache
	Body []byte
}

func NewS3ObjectCache(object *s3.GetObjectOutput) (*S3ObjectCache, error) {
	data, err := io.ReadAll(object.Body)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := object.Body.Close(); err != nil {
			logrus.Errorln("Error closing body:", err)
		}
	}()
	object.Body = io.NopCloser(bytes.NewReader(data))
	return &S3ObjectCache{
		S3ObjectAttributeCache: S3ObjectAttributeCache{
			ETag:         *object.ETag,
			LastModified: *object.LastModified,
		},
		Body: data,
	}, nil
}

func (s *S3ObjectCache) ToGetObjectOutput() *s3.GetObjectOutput {
	return &s3.GetObjectOutput{
		Body:         io.NopCloser(bytes.NewReader(s.Body)),
		ETag:         &s.ETag,
		LastModified: &s.LastModified,
	}
}

func (s *S3Client) setFlesh(key string) {
	err := s.cache.Set(s.cacheContext, "flesh_"+key, nil, store.WithExpiration(time.Duration(s.cfg.S3.FleshTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 flesh cache set error", err)
	}
}

func (s *S3Client) GetCacheObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
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
		params.IfModifiedSince = &data.LastModified
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
	err = s.cache.Set(s.cacheContext, "attr_"+key, &data.S3ObjectAttributeCache, store.WithExpiration(time.Duration(s.cfg.S3.CacheTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 cache attr set error", err)
	}
	s.setFlesh(key)
	return res, nil
}

func NewS3ClientWithObjectCache(s3Client *s3.Client, cache *marshaler.Marshaler, cfg *config.Config) *S3Client {
	return &S3Client{
		Client:       s3Client,
		cacheContext: context.Background(),
		cache:        cache,
		cfg:          cfg,
	}
}

func (s *S3Client) GetObject(key string) (*Object, error) {
	res, err := s.GetCacheObject(context.Background(), &s3.GetObjectInput{
		Bucket: &s.cfg.S3.Bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	return &Object{
		ObjectAttribute: ObjectAttribute{
			ETag:         *res.ETag,
			LastModified: *res.LastModified,
		},
		Body: res.Body,
	}, nil
}

func (s *S3Client) GetCacheObjectAttribute(ctx context.Context, params *s3.GetObjectAttributesInput, optFns ...func(*s3.Options)) (*s3.GetObjectAttributesOutput, error) {
	if !s.cfg.Cache.Activated {
		return s.Client.GetObjectAttributes(ctx, params, optFns...)
	}
	key := *params.Bucket + "/" + *params.Key

	data := &S3ObjectAttributeCache{}
	_, err := s.cache.Get(s.cacheContext, "attr_"+key, data)
	if err == nil {
		logrus.Infoln("S3 attr cache hit", key)
		_, err = s.cache.Get(s.cacheContext, "flesh_"+key, nil)
		if err == nil {
			return &s3.GetObjectAttributesOutput{
				ETag:         &data.ETag,
				LastModified: &data.LastModified,
			}, nil
		}
	}

	res, err := s.Client.GetObjectAttributes(ctx, params, optFns...)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) && re.ResponseError.HTTPStatusCode() == 304 {
			logrus.Infoln("S3 attr not modified", key)
			err = nil
			s.setFlesh(key)
		}
		return res, err
	}

	data = &S3ObjectAttributeCache{
		ETag:         *res.ETag,
		LastModified: *res.LastModified,
	}
	err = s.cache.Set(s.cacheContext, "attr_"+key, data, store.WithExpiration(time.Duration(s.cfg.S3.CacheTime)*time.Second))
	if err != nil {
		logrus.Errorln("S3 attr cache set error", err)
	}
	s.setFlesh(key)
	return res, nil
}

func (s *S3Client) GetObjectAttribute(key string) (*ObjectAttribute, error) {
	res, err := s.GetCacheObjectAttribute(context.Background(), &s3.GetObjectAttributesInput{
		Bucket: &s.cfg.S3.Bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	return &ObjectAttribute{
		ETag:         *res.ETag,
		LastModified: *res.LastModified,
	}, nil
}
