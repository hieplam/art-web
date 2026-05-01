# Layer-B integration tests

Runs the full round-trip described in spec §8.6.2 case 15.

**Prerequisites:**

1. Plan 1's API running locally with `APP_ENV=test`: `cd ../api && APP_ENV=test go run ./cmd/api` with `WORKER_SIGNING_KEY`, `JWT_SIGNING_KEY`, `DATABASE_URL`, `CDN_ORIGIN=http://localhost:8787`, and S3/R2 credentials exported. `APP_ENV=test` registers the `POST /dev/seed` endpoint, but **storage must still be shared with the Worker** for this Layer-B round-trip: set `S3_ENDPOINT` to the Cloudflare R2 S3 endpoint for the same dev bucket bound to wrangler, plus `R2_ACCESS_KEY_ID`, `R2_ACCESS_KEY_SECRET`, and `R2_BUCKET`. Do not leave `S3_ENDPOINT` unset here; that selects localfs for API-only tests and the Worker will 404 because it reads from R2.
2. This Worker running locally: `cd .. && npm run dev` (wrangler dev). Same `WORKER_SIGNING_KEY`. R2 bound to the **same dev bucket** the API writes via S3/R2 credentials; the `[images]` binding requires a Cloudflare account — wrangler 4.x supports an `experimental_remote = true` flag on the IMAGES binding (in `wrangler.toml` under `[dev]`) to proxy through real Cloudflare Images. Pin to `wrangler@^4.44.0` (per contracts §13.3). If no Cloudflare R2 account is available, skip this Layer-B test locally and use Plan 3's MinIO-backed compose harness for storage-sharing coverage; compose does not prove real Cloudflare Images resizing.
3. Mint an owner JWT via the test-only seed endpoint:

```bash
SEED=$(curl -fsS -X POST http://localhost:8080/dev/seed)
ART_OWNER_JWT=$(echo "$SEED" | jq -r .aliceCookie | sed 's/^auth=//')
```

**Run:**

```bash
ART_API_BASE=http://localhost:8080 \
ART_CDN_BASE=http://localhost:8787 \
ART_OWNER_JWT=$ART_OWNER_JWT \
npm run test:integration
```

**Regenerating the fixture:**

```bash
node -e "
const sharp = require('./node_modules/sharp');
sharp({ create: { width: 2400, height: 1600, channels: 3, background: { r: 70, g: 130, b: 180 } } })
  .jpeg({ quality: 85 }).toFile('./test_integration/fixtures/2400px.jpg');
"
```
