package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdimage "image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

// DevSeed handles the POST /dev/seed endpoint, only active when AppEnv == "test".
type DevSeed struct {
	AppEnv   string
	Users    *user.Repo
	Artworks *artwork.Repo
	Tags     *artwork.TagsRepo
	Images   *image.Repo
	Store    storage.Storage
	JWT      *auth.JWT
	Cookie   auth.CookieOpts
}

type seedResponse struct {
	AliceCookie string `json:"aliceCookie"`
	BobCookie   string `json:"bobCookie"`
	AliceSlug   string `json:"aliceSlug"`
	BobSlug     string `json:"bobSlug"`
	PID         string `json:"pId"`
	QID         string `json:"qId"`
}

func (h *DevSeed) handle(w http.ResponseWriter, r *http.Request) {
	if h.AppEnv != "test" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	suffix := devRandHex(4)
	aliceSlug := "alice-" + suffix
	bobSlug := "bob-" + suffix
	aliceID, err := h.Users.UpsertOAuth(ctx, "test", "alice-"+suffix, "alice@test", aliceSlug, "")
	if err != nil {
		writeSeedErr(w, err)
		return
	}
	bobID, err := h.Users.UpsertOAuth(ctx, "test", "bob-"+suffix, "bob@test", bobSlug, "")
	if err != nil {
		writeSeedErr(w, err)
		return
	}

	pID, err := h.Artworks.Create(ctx, aliceID, "Public P", "Seeded public artwork P", "public")
	if err != nil {
		writeSeedErr(w, err)
		return
	}
	qID, err := h.Artworks.Create(ctx, aliceID, "Private Q", "Seeded private artwork Q", "private")
	if err != nil {
		writeSeedErr(w, err)
		return
	}

	if err := h.Tags.SetTags(ctx, pID, []string{"t"}); err != nil {
		writeSeedErr(w, err)
		return
	}
	if err := h.Tags.SetTags(ctx, qID, []string{"t"}); err != nil {
		writeSeedErr(w, err)
		return
	}

	if err := h.attachSeedImage(ctx, pID, "public", "seed-p", 0); err != nil {
		writeSeedErr(w, err)
		return
	}
	if err := h.attachSeedImage(ctx, qID, "private", "seed-q", 0); err != nil {
		writeSeedErr(w, err)
		return
	}

	if many, _ := strconv.Atoi(r.URL.Query().Get("many")); many > 0 {
		const maxMany = 200
		if many > maxMany {
			many = maxMany
		}
		for i := 0; i < many; i++ {
			id, err := h.Artworks.Create(ctx, aliceID, fmt.Sprintf("Bulk %d", i), "Seeded bulk artwork", "public")
			if err != nil {
				writeSeedErr(w, err)
				return
			}
			if err := h.attachSeedImage(ctx, id, "public", fmt.Sprintf("seed-bulk-%d", i), 0); err != nil {
				writeSeedErr(w, err)
				return
			}
		}
	}

	aliceJWT, err := h.JWT.Issue(aliceID, time.Hour)
	if err != nil {
		writeSeedErr(w, err)
		return
	}
	bobJWT, err := h.JWT.Issue(bobID, time.Hour)
	if err != nil {
		writeSeedErr(w, err)
		return
	}

	out := seedResponse{
		AliceCookie: "auth=" + aliceJWT,
		BobCookie:   "auth=" + bobJWT,
		AliceSlug:   aliceSlug,
		BobSlug:     bobSlug,
		PID:         pID,
		QID:         qID,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

const devSeedDim = 64

// devSeedPNG is a 64×64 solid warm-sand PNG used as a visible placeholder in local dev.
var devSeedPNG = func() []byte {
	img := stdimage.NewNRGBA(stdimage.Rect(0, 0, devSeedDim, devSeedDim))
	warm := color.NRGBA{R: 196, G: 168, B: 130, A: 255}
	for y := range devSeedDim {
		for x := range devSeedDim {
			img.SetNRGBA(x, y, warm)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}()

func (h *DevSeed) attachSeedImage(ctx context.Context, artworkID, visibility, clientID string, position int) error {
	imageID := uuid.NewString()
	key := fmt.Sprintf("%s/%s/%s.png", visibility, artworkID, imageID)
	if err := h.Store.Put(ctx, key, bytes.NewReader(devSeedPNG), "image/png"); err != nil {
		return err
	}
	sum := sha256.Sum256(devSeedPNG)
	_, err := h.Images.Insert(ctx, image.InsertInput{
		ID: imageID, ArtworkID: artworkID, ClientImageID: clientID, ContentType: "image/png",
		StorageKey: key, SourceSHA256: hex.EncodeToString(sum[:]),
		Position: position, Width: devSeedDim, Height: devSeedDim, ByteSize: len(devSeedPNG),
		Blurhash: "L00000fQfQfQfQfQfQfQfQfQfQfQ",
	})
	return err
}

func writeSeedErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(500)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "seed_failed", "message": err.Error()})
}

func devRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

