# Seed images

Drop JPEG (`.jpg`, `.jpeg`) or PNG (`.png`) files into this directory.
On the next `go build`, `devseed.go` will embed them and use them in place
of the procedural fallback when serving `POST /dev/seed`.

## Replacing with your own (e.g. grok.com/imagine)

1. Generate or download images with the aesthetic you want.
2. Save them here as any filename. Numeric prefixes (`00.jpg`, `01.jpg`, …)
   are tidy but not required — the loader picks images by hash, not name.
3. Rebuild the API: `cd api && go build ./...`
4. Re-seed: `curl -X POST 'http://localhost:8080/dev/seed?many=50'`

This file is ignored at runtime (only `.jpg` / `.jpeg` / `.png` are loaded).
