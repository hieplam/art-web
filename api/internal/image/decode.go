package image

import (
	"bytes"
	stdimage "image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/buckket/go-blurhash"
)

type DecodeResult struct {
	Width, Height int
	Blurhash      string
}

func DecodeAndBlurhash(r io.Reader, _ string) (*DecodeResult, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	img, _, err := stdimage.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	bb := img.Bounds()
	hash, err := blurhash.Encode(4, 3, img)
	if err != nil {
		return nil, err
	}
	return &DecodeResult{Width: bb.Dx(), Height: bb.Dy(), Blurhash: hash}, nil
}
