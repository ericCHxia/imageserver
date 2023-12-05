package main

import "github.com/h2non/bimg"

type ImageRequest struct {
	bimg.Options
	FilePath string `json:"path" form:"path" binding:"required"`
}

func newImageRequest() *ImageRequest {
	return &ImageRequest{}
}
