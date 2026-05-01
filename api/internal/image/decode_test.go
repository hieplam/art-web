package image_test

import (
	"bytes"
	"os"
	"testing"

	"local/art-web/api/internal/image"
)

func TestDecodeAndBlurhash_JPEG(t *testing.T) {
	raw, _ := os.ReadFile("testdata/sample.jpg")
	out, err := image.DecodeAndBlurhash(bytes.NewReader(raw), "image/jpeg")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Width != 50 || out.Height != 50 {
		t.Fatalf("dims: %dx%d", out.Width, out.Height)
	}
	if len(out.Blurhash) < 10 {
		t.Fatalf("blurhash too short: %q", out.Blurhash)
	}
}
