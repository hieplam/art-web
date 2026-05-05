# Observed error codes (Phase 0 snapshot)

This table is the canonical input to Phase 1's `WriteError` switch (spec §7.4)
and validator translator (§7.5). Every code listed here is byte-locked into
a golden file in `golden/`. Phase 1 must emit exactly these codes for these
statuses.

## Generation

Re-run after any new golden capture:

```bash
cd api
for f in internal/httpapi/contract/golden/*.json; do
  jq -r '
    {status: .status,
     err:    (.body | try fromjson | .error // empty)}
    | select(.err != "")
    | "\(.status)\t\(.err)"
  ' "$f"
done | sort -u
```

## Captured pairs (PR 0.8 baseline — 56 cells, 13 unique error codes)

| Status | Error code | Shape | Cells locking it |
|---|---|---|---|
| 400 | `bad_cover_position` | A | `patch_owner_bad_cover_position` |
| 400 | `bad_cursor` | **B** (with `message`) | `feed_anon_bad_cursor`, `user_profile_bad_cursor` |
| 400 | `bad_json` | A | `create_owner_bad_json`, `patch_owner_bad_json` |
| 400 | `bad_state` | A | `auth_callback_bad_state_400` |
| 400 | `bad_visibility` | A | `create_owner_bad_visibility`, `patch_owner_bad_visibility` |
| 400 | `file_count_mismatch` | A | `upload_owner_400_file_count_mismatch` |
| 400 | `manifest_required` | A | `upload_owner_400_no_manifest` |
| 401 | `unauthorized` | A | `create_anon_401`, `delete_anon_401`, `me_anon_401`, `patch_anon_401`, `upload_anon_401` |
| 404 | `not_found` | A | `delete_other_*`, `delete_owner_missing_404`, `get_artwork_*_404`, `patch_other_*`, `upload_other_404`, `user_profile_missing_slug` |
| 404 | `unknown_provider` | A | `auth_callback_unknown_provider_404`, `auth_unknown_provider_404` |
| 415 | `unsupported_media_type` | A | `upload_owner_415_non_multipart` |
| 422 | `decode_failed` | A | `upload_owner_decode_failed_422` |
| 502 | `exchange_failed` | A | `auth_callback_exchange_failed_502` |

## Status surface (every status appearing in any golden)

| Status | Cells | Notes |
|---|---|---|
| 200 | feed, get_artwork (success), me, tag, user_profile, healthz, auth_unknown_provider | success bodies |
| 201 | `create_owner_minimal`, `create_owner_full`, `create_owner_default_visibility` | `{"id": "<UUID>"}` |
| 204 | `logout_*`, `patch_owner_*` (success), `patch_owner_full_204` | empty body |
| 302 | `auth_callback_success_302`, `auth_google_start_anon_302` | redirects |
| 400 | (see error table above) |  |
| 401 | (see error table above) |  |
| 404 | (see error table above) |  |
| 415 | (see error table above) |  |
| 422 | (see error table above) |  |
| 502 | (see error table above) |  |

## Codes from spec §3 NOT in goldens (gaps Phase 1 must still emit)

These error codes appear in production handlers but the contract matrix doesn't
exercise them organically. Phase 1's `WriteError` mapping must include them
even though no golden pins their bytes:

| Status | Code | Why no golden |
|---|---|---|
| 500 | `create_failed`, `patch_failed`, `flip_failed`, `tag_failed`, `delete_failed`, `list_failed`, `user_lookup_failed`, `user_failed` | DB-failure paths; require fault injection per spec §5.6 |
| 500 | `sign_failed`, `upsert_failed` | downstream-failure paths in auth callback |
| 500 | `upload_failed` | DB-failure path in image upload |
| 400 | `open_file` | requires multipart File.Open() error; not reproducible without library-level fault injection |
| 412 | `position_taken` | covered by `image/handler_test.go:TestUploadCase19_ConcurrentSamePosition`; not in matrix |
| 422 | `too_large` | covered by `httpapi/error_codes_test.go:TestErrors_ImageUpload_422_TooLarge` |
| 422 | `content_type_mismatch` | **B** (with `message`); covered by `image/handler_test.go:TestUploadContentTypeMismatch_Returns422` |
| 409 | `fingerprint_mismatch` | **B** (with `message`); covered by `image/handler_test.go:TestUploadFingerprintMismatch_*` |
| 400 | ParseManifest free-form (Shape-A quirky) | covered by `httpapi/error_codes_test.go:TestErrors_ImageUpload_400_ParseManifestQuirk` — not byte-locked because the quirky shape is `{"error": <free-form>}` |

Phase 1's `WriteError` switch MUST cover every code in both tables (in-golden +
not-in-golden) to preserve the §1 non-goal "no API contract changes."

## Forbidden statuses (per §5.2.0a, asserted by `forbidden_status_test.go`)

These statuses are explicitly absent from all goldens and a meta-test fails CI
if any golden adds them:

| Status | Reason |
|---|---|
| 403 | Non-owner access collapses to 404 (spec §7.2.1). |
| 410 | No current handler emits 410. |
| 451 | No current handler emits 451. |
| 511 | No current handler emits 511. |
| 409 (without `fingerprint_mismatch` body) | Translated domain errors (`ErrAlreadyPublished`) must remain 204, not 409. |
