package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ManifestEntry struct {
	ClientImageID string `json:"client_image_id"`
	Position      int    `json:"position"`
	ContentType   string `json:"content_type"`
}

var allowedTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
}

func ExtFor(ct string) (string, bool) {
	e, ok := allowedTypes[ct]
	return e, ok
}

func ParseManifest(r io.Reader) ([]ManifestEntry, error) {
	var out []ManifestEntry
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("manifest: empty")
	}
	seenPos := map[int]bool{}
	seenID := map[string]bool{}
	for i, e := range out {
		if e.ClientImageID == "" {
			return nil, fmt.Errorf("manifest[%d]: client_image_id required", i)
		}
		if _, ok := allowedTypes[e.ContentType]; !ok {
			return nil, fmt.Errorf("manifest[%d]: unsupported content_type %q", i, e.ContentType)
		}
		if seenPos[e.Position] {
			return nil, fmt.Errorf("manifest[%d]: duplicate position %d", i, e.Position)
		}
		if seenID[e.ClientImageID] {
			return nil, fmt.Errorf("manifest[%d]: duplicate client_image_id", i)
		}
		seenPos[e.Position] = true
		seenID[e.ClientImageID] = true
	}
	return out, nil
}
