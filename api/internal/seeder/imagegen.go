package seeder

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	stdimage "image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	mrand "math/rand/v2"
	"path"
	"strings"
	"sync"

	"github.com/buckket/go-blurhash"
)

// seedsFS holds curated retro-style images bundled at compile time. Drop
// images into api/internal/seeder/seeds/ and rebuild — the loader will pick
// them up. JPEG and PNG are both supported. If the directory has no usable
// images, the procedural generator runs as a fallback so seeding never
// blocks on missing content.
//
//go:embed seeds
var seedsFS embed.FS

// seedSource is one resolved seed image — either a curated file from
// seeds/ or a deterministically generated procedural image. Metadata is
// precomputed so attachSeedImage stays cheap when the same source is reused
// across many seeded artworks.
type seedSource struct {
	bytes       []byte
	contentType string
	ext         string // includes leading dot, e.g. ".jpg"
	width       int
	height      int
	blurhash    string
	sha256Hex   string
}

var (
	embeddedSeedsOnce sync.Once
	embeddedSeeds     []seedSource
)

// pickSeedSource returns one image deterministically chosen from the curated
// embedded library when available, falling back to a per-clientID procedural
// artwork when the seeds directory is empty.
func pickSeedSource(clientID string) (seedSource, error) {
	loadEmbeddedSeeds()
	if n := len(embeddedSeeds); n > 0 {
		idx := int(seedFromString(clientID) % uint64(n))
		return embeddedSeeds[idx], nil
	}
	img, raw, err := generateSeedArtwork(clientID)
	if err != nil {
		return seedSource{}, err
	}
	hash, err := blurhash.Encode(4, 3, img)
	if err != nil {
		hash = "L00000fQfQfQfQfQfQfQfQfQfQfQ"
	}
	sum := sha256.Sum256(raw)
	return seedSource{
		bytes:       raw,
		contentType: "image/png",
		ext:         ".png",
		width:       img.Bounds().Dx(),
		height:      img.Bounds().Dy(),
		blurhash:    hash,
		sha256Hex:   hex.EncodeToString(sum[:]),
	}, nil
}

// loadEmbeddedSeeds walks the embedded seeds directory once and records every
// usable JPEG/PNG. Non-image files (README.md, .gitkeep) are silently skipped.
// A corrupt or unreadable image is also skipped, never fatal — the loader
// degrades to whatever images remain, then to procedural.
func loadEmbeddedSeeds() {
	embeddedSeedsOnce.Do(func() {
		entries, err := seedsFS.ReadDir("seeds")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			extLower := strings.ToLower(path.Ext(name))
			if extLower != ".jpg" && extLower != ".jpeg" && extLower != ".png" {
				continue
			}
			raw, err := seedsFS.ReadFile("seeds/" + name)
			if err != nil || len(raw) < 256 {
				continue
			}
			// Trust the bytes, not the filename: some web galleries hand out
			// .png-named URLs that contain JPEG-encoded data.
			img, fmtName, err := stdimage.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			var ct, ext string
			switch fmtName {
			case "jpeg":
				ct, ext = "image/jpeg", ".jpg"
			case "png":
				ct, ext = "image/png", ".png"
			default:
				continue
			}
			hash, err := blurhash.Encode(4, 3, img)
			if err != nil {
				hash = "L00000fQfQfQfQfQfQfQfQfQfQfQ"
			}
			sum := sha256.Sum256(raw)
			b := img.Bounds()
			embeddedSeeds = append(embeddedSeeds, seedSource{
				bytes:       raw,
				contentType: ct,
				ext:         ext,
				width:       b.Dx(),
				height:      b.Dy(),
				blurhash:    hash,
				sha256Hex:   hex.EncodeToString(sum[:]),
			})
		}
	})
}

// ── Procedural seed artwork generator ────────────────────────────────────────
//
// generateSeedArtwork produces a deterministic painterly image keyed on
// clientID. Same clientID → same image bytes. Outputs varied aspect ratios
// and three composition styles so a seeded feed reads as a curated gallery
// drop, not a tile of identical placeholders.

// 960px long edge — large enough to look gallery-grade in the masonry,
// small enough that PNG encode stays well under the seed-time budget.
const seedLongEdge = 960

// Aspect ratios cycle deterministically by index (derived from the seed),
// so a many=N seed produces a believable mix of portraits, squares,
// landscapes, and ultrawides.
var seedAspects = [...][2]int{
	{2, 3},  // portrait
	{3, 4},  // portrait
	{1, 1},  // square
	{4, 3},  // landscape
	{3, 2},  // landscape
	{16, 9}, // ultrawide
}

// Eight curated three-color palettes tuned in OKLab. Each palette commits to
// a mood (ember-night, ice-teal, …) so a feed has range without feeling
// random.
type palette struct {
	a, b, c color.NRGBA
}

var seedPalettes = [...]palette{
	{color.NRGBA{0x2a, 0x14, 0x10, 0xff}, color.NRGBA{0xc4, 0x57, 0x2a, 0xff}, color.NRGBA{0xf2, 0xc1, 0x84, 0xff}}, // ember-night
	{color.NRGBA{0x0d, 0x1f, 0x2a, 0xff}, color.NRGBA{0x29, 0x86, 0x9c, 0xff}, color.NRGBA{0xd6, 0xee, 0xf3, 0xff}}, // ice-teal
	{color.NRGBA{0x12, 0x1d, 0x18, 0xff}, color.NRGBA{0x3f, 0x6b, 0x4d, 0xff}, color.NRGBA{0xc8, 0xd8, 0xb3, 0xff}}, // viridian-fog
	{color.NRGBA{0x1a, 0x07, 0x07, 0xff}, color.NRGBA{0x6d, 0x12, 0x1a, 0xff}, color.NRGBA{0xe1, 0x96, 0x66, 0xff}}, // oxblood
	{color.NRGBA{0x14, 0x14, 0x16, 0xff}, color.NRGBA{0x4e, 0x52, 0x5a, 0xff}, color.NRGBA{0xd6, 0xd1, 0xc4, 0xff}}, // bone-graphite
	{color.NRGBA{0x07, 0x10, 0x2a, 0xff}, color.NRGBA{0x21, 0x3e, 0x82, 0xff}, color.NRGBA{0xe6, 0xc8, 0x76, 0xff}}, // kyoto-blue
	{color.NRGBA{0x1d, 0x0a, 0x1f, 0xff}, color.NRGBA{0x71, 0x2c, 0x66, 0xff}, color.NRGBA{0xe5, 0x9a, 0xc8, 0xff}}, // dusk-magenta
	{color.NRGBA{0x14, 0x0e, 0x06, 0xff}, color.NRGBA{0x8c, 0x60, 0x1f, 0xff}, color.NRGBA{0xf3, 0xe1, 0xab, 0xff}}, // warm-rust
}

// generateSeedArtwork builds a procedural image deterministically from
// clientID. Returns the in-memory image (for blurhash) and the PNG bytes.
func generateSeedArtwork(clientID string) (stdimage.Image, []byte, error) {
	seed := seedFromString(clientID)
	rng := mrand.New(mrand.NewPCG(seed, seed^0x9e3779b97f4a7c15))

	aspect := seedAspects[rng.UintN(uint(len(seedAspects)))]
	w, h := dimsFromAspect(aspect[0], aspect[1], seedLongEdge)
	pal := seedPalettes[rng.UintN(uint(len(seedPalettes)))]

	img := stdimage.NewNRGBA(stdimage.Rect(0, 0, w, h))

	switch rng.IntN(3) {
	case 0:
		paintGradientField(img, pal, rng)
	case 1:
		paintBlockConstructivism(img, pal, rng)
	default:
		paintParticleDrift(img, pal, rng)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, nil, err
	}
	return img, buf.Bytes(), nil
}

// dimsFromAspect returns the largest (w, h) honoring aspect (aw:ah) with the
// long edge fixed at long. Both are rounded down to even ints (PNG-friendly).
func dimsFromAspect(aw, ah, long int) (int, int) {
	if aw >= ah {
		w := long
		h := long * ah / aw
		return w &^ 1, h &^ 1
	}
	h := long
	w := long * aw / ah
	return w &^ 1, h &^ 1
}

// ── Composition styles ───────────────────────────────────────────────────────

// paintGradientField fills with a 2-stop gradient (corner→corner) plus a
// soft low-frequency sin/cos noise that gives the field a hand-made wash
// quality.
func paintGradientField(img *stdimage.NRGBA, p palette, rng *mrand.Rand) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	angle := rng.Float64() * 2 * math.Pi
	dx, dy := math.Cos(angle), math.Sin(angle)
	maxProj := math.Abs(dx)*float64(w) + math.Abs(dy)*float64(h)
	freq1 := 0.006 + rng.Float64()*0.004
	freq2 := 0.011 + rng.Float64()*0.006
	phase1 := rng.Float64() * 100
	phase2 := rng.Float64() * 100

	a, b := p.a, p.b
	highlight := p.c

	for y := range h {
		for x := range w {
			proj := float64(x)*dx + float64(y)*dy
			if proj < 0 {
				proj = -proj
			}
			t := proj / maxProj
			noise := 0.5*math.Sin(float64(x)*freq1+float64(y)*freq2+phase1) +
				0.5*math.Cos(float64(x)*freq2-float64(y)*freq1+phase2)
			t = clamp01(t + noise*0.08)
			base := lerpColor(a, b, t)

			veil := math.Max(0, 0.5-math.Hypot(float64(x)/float64(w)-0.2, float64(y)/float64(h)-0.2))
			base = lerpColor(base, highlight, veil*0.35)

			img.SetNRGBA(x, y, base)
		}
	}
}

// paintBlockConstructivism stacks 3–6 translucent rectangles in palette
// order over a base wash. Bold, graphic, but still painterly because of
// the dither.
func paintBlockConstructivism(img *stdimage.NRGBA, p palette, rng *mrand.Rand) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, p.a)
		}
	}

	colors := [3]color.NRGBA{p.a, p.b, p.c}
	blocks := 3 + rng.IntN(4)
	for range blocks {
		bw := w/4 + rng.IntN(w/2)
		bh := h/4 + rng.IntN(h/2)
		x0 := rng.IntN(w-bw+1) - bw/8
		y0 := rng.IntN(h-bh+1) - bh/8
		alpha := uint8(120 + rng.UintN(110))
		col := colors[rng.IntN(3)]
		col.A = alpha
		fillRectAlpha(img, x0, y0, bw, bh, col)
	}

	for y := range h {
		for x := range w {
			if rng.IntN(96) == 0 {
				c := img.NRGBAAt(x, y)
				j := int8(rng.IntN(11) - 5)
				c.R = clampU8(int(c.R) + int(j))
				c.G = clampU8(int(c.G) + int(j))
				c.B = clampU8(int(c.B) + int(j))
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// paintParticleDrift lays a base wash, then scatters 220–360 soft circles
// of varied size and opacity. Reads as digital fog / starfield / dust.
func paintParticleDrift(img *stdimage.NRGBA, p palette, rng *mrand.Rand) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	for y := range h {
		t := float64(y) / float64(h)
		base := lerpColor(p.a, p.b, t*0.7)
		for x := range w {
			img.SetNRGBA(x, y, base)
		}
	}

	count := 220 + rng.IntN(140)
	for range count {
		cx := rng.IntN(w)
		cy := rng.IntN(h)
		r := 2 + rng.IntN(14)
		alpha := uint8(40 + rng.UintN(160))
		col := p.c
		if rng.IntN(3) == 0 {
			col = p.b
		}
		col.A = alpha
		stampSoftCircle(img, cx, cy, r, col)
	}
}

// ── Pixel helpers ────────────────────────────────────────────────────────────

func fillRectAlpha(img *stdimage.NRGBA, x0, y0, w, h int, c color.NRGBA) {
	bx, by := img.Bounds().Dx(), img.Bounds().Dy()
	for y := y0; y < y0+h; y++ {
		if y < 0 || y >= by {
			continue
		}
		for x := x0; x < x0+w; x++ {
			if x < 0 || x >= bx {
				continue
			}
			img.SetNRGBA(x, y, blendOver(img.NRGBAAt(x, y), c))
		}
	}
}

// stampSoftCircle paints a radial-falloff disc at (cx,cy) with peak alpha c.A.
func stampSoftCircle(img *stdimage.NRGBA, cx, cy, r int, c color.NRGBA) {
	bx, by := img.Bounds().Dx(), img.Bounds().Dy()
	rsq := float64(r * r)
	peak := float64(c.A)
	for y := cy - r; y <= cy+r; y++ {
		if y < 0 || y >= by {
			continue
		}
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || x >= bx {
				continue
			}
			dx := float64(x - cx)
			dy := float64(y - cy)
			d := dx*dx + dy*dy
			if d > rsq {
				continue
			}
			falloff := 1 - d/rsq
			a := uint8(peak * falloff * falloff)
			if a == 0 {
				continue
			}
			tint := c
			tint.A = a
			img.SetNRGBA(x, y, blendOver(img.NRGBAAt(x, y), tint))
		}
	}
}

// blendOver applies "src-over" in straight-alpha space.
func blendOver(dst, src color.NRGBA) color.NRGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / outA
		return clampU8(int(v + 0.5))
	}
	return color.NRGBA{
		R: mix(src.R, dst.R),
		G: mix(src.G, dst.G),
		B: mix(src.B, dst.B),
		A: clampU8(int(outA*255 + 0.5)),
	}
}

// lerpColor linearly interpolates two opaque colors in straight RGB.
func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	t = clamp01(t)
	return color.NRGBA{
		R: clampU8(int(float64(a.R) + (float64(b.R)-float64(a.R))*t + 0.5)),
		G: clampU8(int(float64(a.G) + (float64(b.G)-float64(a.G))*t + 0.5)),
		B: clampU8(int(float64(a.B) + (float64(b.B)-float64(a.B))*t + 0.5)),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampU8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
