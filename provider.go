package main

import (
	"io"
	"time"
)

type Object struct {
	ObjectAttribute
	Body io.ReadCloser
}

type ObjectAttribute struct {
	ETag         string
	LastModified time.Time
}

type Provider interface {
	GetObject(key string) (*Object, error)
	GetObjectAttribute(key string) (*ObjectAttribute, error)
}
