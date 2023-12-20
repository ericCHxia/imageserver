package main

import (
	"fmt"
	"github.com/davidbyttow/govips/v2/vips"
	"math"
)

type Options struct {
	Width  int
	Height int
	Expand bool
	Type   vips.ImageType
}

var ImageTypes = map[vips.ImageType]string{
	vips.ImageTypeJPEG: "jpeg",
	vips.ImageTypePNG:  "png",
	vips.ImageTypeWEBP: "webp",
	vips.ImageTypeAVIF: "avif",
}

func NewOption() *Options {
	return &Options{
		Width:  0,
		Height: 0,
		Expand: false,
		Type:   vips.ImageTypeUnknown,
	}
}

func exportImage(image *vips.ImageRef, option *Options) ([]byte, error) {
	if option.Type == vips.ImageTypeUnknown {
		option.Type = image.Format()
	}
	switch option.Type {
	case vips.ImageTypeJPEG:
		param := vips.NewJpegExportParams()
		out, _, err := image.ExportJpeg(param)
		return out, err
	case vips.ImageTypePNG:
		param := vips.NewPngExportParams()
		out, _, err := image.ExportPng(param)
		return out, err
	case vips.ImageTypeWEBP:
		param := vips.NewWebpExportParams()
		out, _, err := image.ExportWebp(param)
		return out, err
	case vips.ImageTypeAVIF:
		param := vips.NewAvifExportParams()
		out, _, err := image.ExportAvif(param)
		return out, err
	default:
		return nil, fmt.Errorf("unsupported image type: %v", option.Type)
	}
}

func Process(image *vips.ImageRef, option *Options) ([]byte, error) {
	if option.Width > 0 || option.Height > 0 {
		hscale := float64(option.Width) / float64(image.Width())
		vscale := float64(option.Height) / float64(image.Height())
		if option.Width == 0 {
			hscale = vscale
		}
		if option.Height == 0 {
			vscale = hscale
		}
		scale := math.Min(hscale, vscale)
		if scale < 1.0 || option.Expand {
			err := image.Resize(scale, vips.KernelLanczos3)
			if err != nil {
				return nil, err
			}
		}
	}
	return exportImage(image, option)
}

type ImageRequest struct {
	*Options
	FilePath string `json:"path" form:"path" binding:"required"`
}

func newImageRequest() *ImageRequest {
	return &ImageRequest{
		Options: NewOption(),
	}
}
