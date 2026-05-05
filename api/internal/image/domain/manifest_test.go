package domain_test

import (
	"strings"
	"testing"

	imagedomain "local/art-web/api/internal/image/domain"
)

func TestParseManifest_OK(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K2","position":1,"content_type":"image/png"}]`
	out, err := imagedomain.ParseManifest(strings.NewReader(js))
	if err != nil || len(out) != 2 {
		t.Fatalf("got %v, %v", out, err)
	}
}

func TestParseManifest_RejectsUnsupportedContentType(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/gif"}]`
	if _, err := imagedomain.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected reject")
	}
}

func TestParseManifest_RejectsDuplicatePosition(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K2","position":0,"content_type":"image/jpeg"}]`
	if _, err := imagedomain.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected dup-position reject")
	}
}

func TestParseManifest_RejectsDuplicateClientID(t *testing.T) {
	js := `[{"client_image_id":"K","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K","position":1,"content_type":"image/jpeg"}]`
	if _, err := imagedomain.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected dup-clientid reject")
	}
}

func TestParseManifest_RejectsInvalidJSON(t *testing.T) {
	if _, err := imagedomain.ParseManifest(strings.NewReader(`not json`)); err == nil {
		t.Fatal("expected JSON decode error")
	}
}

func TestParseManifest_RejectsEmptyArray(t *testing.T) {
	if _, err := imagedomain.ParseManifest(strings.NewReader(`[]`)); err == nil {
		t.Fatal("expected empty manifest error")
	}
}

func TestParseManifest_RejectsMissingClientImageID(t *testing.T) {
	js := `[{"position":0,"content_type":"image/jpeg"}]`
	if _, err := imagedomain.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected missing client_image_id error")
	}
}
