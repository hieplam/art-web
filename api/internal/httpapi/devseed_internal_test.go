package httpapi

import (
	"bytes"
	"image/png"
	"testing"
)

// TestGenerateSeedArtwork_Deterministic asserts same clientID → same image bytes.
// Reproducibility is required so test fixtures and storage_keys remain stable.
func TestGenerateSeedArtwork_Deterministic(t *testing.T) {
	_, a, err := generateSeedArtwork("seed-bulk-1")
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	_, b, err := generateSeedArtwork("seed-bulk-1")
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("identical seed produced different bytes: %d vs %d", len(a), len(b))
	}
}

// TestGenerateSeedArtwork_DistinctSeedsDiffer guards against an RNG misuse that
// would collapse all bulk seeds into the same image — the precise regression
// the previous warm-sand placeholder had.
func TestGenerateSeedArtwork_DistinctSeedsDiffer(t *testing.T) {
	_, a, err := generateSeedArtwork("seed-bulk-1")
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := generateSeedArtwork("seed-bulk-2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("distinct clientIDs produced identical bytes; RNG seeding is broken")
	}
}

// TestGenerateSeedArtwork_DecodesAsPNG confirms the encoder output is a valid
// PNG and the decoded dimensions agree with the in-memory image bounds.
func TestGenerateSeedArtwork_DecodesAsPNG(t *testing.T) {
	img, raw, err := generateSeedArtwork("seed-p")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Fatalf("bounds mismatch: encoder=%v decoded=%v", img.Bounds(), decoded.Bounds())
	}
	if img.Bounds().Dx() < 200 || img.Bounds().Dy() < 200 {
		t.Fatalf("seed images should be gallery-sized; got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// TestGenerateSeedArtwork_VariedAspectRatios covers the masonry-feel intent:
// across many seeds, we should see at least 3 distinct aspect ratios.
func TestGenerateSeedArtwork_VariedAspectRatios(t *testing.T) {
	seen := map[string]int{}
	for i := range 30 {
		img, _, err := generateSeedArtwork(seedID(i))
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		seen[ratioBucket(w, h)]++
	}
	if len(seen) < 3 {
		t.Fatalf("expected at least 3 distinct aspect buckets across 30 seeds, got %d: %v", len(seen), seen)
	}
}

func seedID(i int) string {
	return "seed-bulk-" + string(rune('a'+i%26)) + string(rune('0'+i%10))
}

func ratioBucket(w, h int) string {
	switch {
	case w*3 == h*2: // 2:3
		return "2:3"
	case w*4 == h*3: // 3:4
		return "3:4"
	case w == h:
		return "1:1"
	case w*3 == h*4: // 4:3
		return "4:3"
	case w*2 == h*3: // 3:2
		return "3:2"
	case w*9 == h*16: // 16:9
		return "16:9"
	}
	if w > h {
		return "wide-other"
	}
	if h > w {
		return "tall-other"
	}
	return "square-other"
}
