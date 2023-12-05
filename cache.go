package main

import (
	"context"
	"github.com/eko/gocache/lib/v4/marshaler"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
)

type cachedWriter struct {
	gin.ResponseWriter
	context context.Context
	status  int
	written bool
	store   *marshaler.Marshaler
	key     []byte
}

type responseCache struct {
	Status int
	Header http.Header
	Data   []byte
}

func (w *cachedWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.written = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *cachedWriter) Write(b []byte) (int, error) {
	ret, err := w.ResponseWriter.Write(b)
	if err == nil && w.Status() < 300 {
		store := w.store
		var cache = &responseCache{}
		if _, err := store.Get(w.context, w.key, cache); err != nil {
			b = append(cache.Data, b...)
		}
		cache.Status = w.Status()
		cache.Header = w.Header()
		cache.Data = b
		err := store.Set(w.context, w.key, cache)
		if err != nil {
			logrus.Errorln("Error caching response", err)
		}
	}
	return ret, err
}
