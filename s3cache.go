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

type S3Provider struct {
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

func (s *S3ObjectCache) ToGetObjectOutput() *s3.GetObjectOutput {
	return &s3.GetObjectOutput{
		Body:         io.NopCloser(bytes.NewReader(s.Body)),
		ETag:         &s.ETag,
		LastModified: &s.LastModified,
	}
}

func NewS3ProviderWithObjectCache(s3Client *s3.Client, cache *marshaler.Marshaler, cfg *config.Config) *S3Provider {
	return &S3Provider{
		Client:       s3Client,
		cacheContext: context.Background(),
		cache:        cache,
		cfg:          cfg,
	}
}

func (s *S3Provider) GetObjectFromCache(key string) (*Object, error) {
	data := &S3ObjectCache{}
	_, err := s.cache.Get(s.cacheContext, key, &data)
	if err != nil {
		return nil, err
	}
	return &Object{
		ObjectAttribute: ObjectAttribute{
			ETag:         data.ETag,
			LastModified: data.LastModified,
		},
		Body: data.Body,
	}, nil
}

func (s *S3Provider) SetObjectToCache(key string, data *Object) error {
	cacheObject := &S3ObjectCache{
		S3ObjectAttributeCache: S3ObjectAttributeCache{
			ETag:         data.ETag,
			LastModified: data.LastModified,
		},
		Body: data.Body,
	}
	err := s.cache.Set(s.cacheContext, key, cacheObject)
	if err != nil {
		return err
	}
	return s.setObjectAttributeToCache(key, &data.ObjectAttribute)
}

func (s *S3Provider) GetObject(key string) (*Object, error) {
	params := &s3.GetObjectInput{
		Bucket: &s.cfg.S3.Bucket,
		Key:    &key,
	}
	CacheKey := *params.Bucket + "/" + *params.Key

	cache, err := s.GetObjectFromCache(CacheKey)
	if err == nil {
		params.IfNoneMatch = &cache.ETag
		params.IfModifiedSince = &cache.LastModified
	}

	res, err := s.Client.GetObject(context.Background(), params)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) && re.ResponseError.HTTPStatusCode() == 304 {
			logrus.Infoln("S3 not modified", key)
			err = nil
			return cache, nil
		}
		return nil, err
	}
	data, err := io.ReadAll(res.Body)
	defer func() {
		if err := res.Body.Close(); err != nil {
			logrus.Errorln("Error closing response body", err)
		}
	}()
	if err != nil {
		return nil, err
	}
	obj := &Object{
		ObjectAttribute: ObjectAttribute{
			ETag:         *res.ETag,
			LastModified: *res.LastModified,
		},
		Body: data,
	}
	err = s.SetObjectToCache(CacheKey, obj)
	if err != nil {
		logrus.Errorln("Error setting cache", err)
	}
	return obj, nil
}

func (s *S3Provider) GetObjectAttributeFromCache(key string) (*ObjectAttribute, error) {
	data := &S3ObjectAttributeCache{}
	_, err := s.cache.Get(s.cacheContext, "attr_"+key, data)
	if err != nil {
		return nil, err
	}
	return &ObjectAttribute{
		ETag:         data.ETag,
		LastModified: data.LastModified,
	}, nil
}

func (s *S3Provider) setObjectAttributeToCache(key string, data *ObjectAttribute) error {
	cacheObject := &S3ObjectAttributeCache{
		ETag:         data.ETag,
		LastModified: data.LastModified,
	}
	err := s.cache.Set(s.cacheContext, "attr_"+key, cacheObject, store.WithExpiration(time.Duration(s.cfg.S3.CacheTime)*time.Second))
	return err
}

func (s *S3Provider) GetObjectAttribute(key string) (*ObjectAttribute, error) {
	params := &s3.GetObjectAttributesInput{
		Bucket: &s.cfg.S3.Bucket,
		Key:    &key,
	}
	CacheKey := *params.Bucket + "/" + *params.Key

	cache, err := s.GetObjectAttributeFromCache(CacheKey)
	if err == nil {
		return cache, nil
	}

	res, err := s.Client.GetObjectAttributes(context.Background(), params)

	if err != nil {
		return nil, err
	}

	attr := &ObjectAttribute{
		ETag:         *res.ETag,
		LastModified: *res.LastModified,
	}
	err = s.setObjectAttributeToCache(CacheKey, attr)
	if err != nil {
		logrus.Errorln("Error setting cache", err)
	}
	return attr, nil
}
