package service_test

import (
	"bytes"
	"os"
	"testing"

	imageservice "local/art-web/api/internal/image/service"
)

func TestDecodeAndBlurhash_JPEG(t *testing.T) {
	raw, _ := os.ReadFile("testdata/sample.jpg")
	out, err := imageservice.DecodeAndBlurhash(bytes.NewReader(raw), "image/jpeg")
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

func TestDecodeAndBlurhash_ReturnsFormat(t *testing.T) {
	cases := []struct {
		file string
		want string
	}{
		{"testdata/sample.jpg", "jpeg"},
		{"testdata/sample.png", "png"},
	}
	for _, tc := range cases {
		raw, _ := os.ReadFile(tc.file)
		out, err := imageservice.DecodeAndBlurhash(bytes.NewReader(raw), "")
		if err != nil {
			t.Fatalf("%s decode: %v", tc.file, err)
		}
		if out.Format != tc.want {
			t.Fatalf("%s: expected Format %q, got %q", tc.file, tc.want, out.Format)
		}
	}
}
