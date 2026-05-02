#!/usr/bin/env bash
# dev-up.sh — bring the art-web e2e stack up, seed it, flush web cache.
#
# Usage:
#   ./dev-up.sh                  # wipe data, full rebuild, seed 50
#   ./dev-up.sh --many=120       # seed 120 instead
#   ./dev-up.sh --keep-data      # iterate without losing the bucket / DB
#   ./dev-up.sh --no-flush       # skip web ISR-cache flush (if you don't need it)
#   ./dev-up.sh --no-build       # reuse images (faster when only seeding changed)

set -euo pipefail

# ── Args ─────────────────────────────────────────────────────────────────────
SEED_COUNT=50
KEEP_DATA=false
NO_FLUSH=false
NO_BUILD=false

for arg in "$@"; do
  case "$arg" in
    --many=*)     SEED_COUNT="${arg#*=}" ;;
    --keep-data)  KEEP_DATA=true ;;
    --no-flush)   NO_FLUSH=true ;;
    --no-build)   NO_BUILD=true ;;
    -h|--help)
      sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      exit 64
      ;;
  esac
done

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/web/docker-compose.e2e.yml"
COMPOSE=(docker compose -f "$COMPOSE_FILE")

# ── Pretty logging ───────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  C_DIM=$'\e[2m'; C_BOLD=$'\e[1m'; C_OK=$'\e[32m'; C_ERR=$'\e[31m'; C_RESET=$'\e[0m'
else
  C_DIM=""; C_BOLD=""; C_OK=""; C_ERR=""; C_RESET=""
fi
step()  { printf "${C_BOLD}▸ %s${C_RESET}\n" "$*"; }
ok()    { printf "${C_OK}✓ %s${C_RESET}\n" "$*"; }
fail()  { printf "${C_ERR}✗ %s${C_RESET}\n" "$*" >&2; exit 1; }

# Block until `probe` exits 0, or fail after `timeout` seconds.
wait_for() {
  local label=$1 probe=$2 timeout=${3:-90}
  local i=0
  printf "  waiting for %s" "$label"
  until eval "$probe" >/dev/null 2>&1; do
    if (( i >= timeout )); then
      printf "\n"
      fail "timed out after ${timeout}s waiting for $label"
    fi
    printf "."
    sleep 1
    i=$((i+1))
  done
  printf " ${C_OK}up${C_RESET} (${C_DIM}${i}s${C_RESET})\n"
}

# ── 1. Optional wipe ─────────────────────────────────────────────────────────
if ! $KEEP_DATA; then
  step "wiping previous containers + volumes"
  "${COMPOSE[@]}" down -v --remove-orphans
fi

# ── 2. Build + boot ──────────────────────────────────────────────────────────
build_flag=()
$NO_BUILD || build_flag=(--build)

step "starting stack ${build_flag[*]:-(no rebuild)}"
"${COMPOSE[@]}" up "${build_flag[@]}" -d --wait

# ── 3. Probe service readiness ───────────────────────────────────────────────
# `--wait` already blocks on healthchecks, but those checks are defined inside
# the compose file and may not match the public endpoints we care about. We
# probe the actually-reachable URLs to be sure.
wait_for "api"      "curl -sf http://localhost:8080/healthz"
wait_for "web"      "curl -sf -o /dev/null http://localhost:3000"
wait_for "worker"   "curl -sf http://localhost:8787/healthz"

# ── 4. Seed ──────────────────────────────────────────────────────────────────
step "seeding $SEED_COUNT artworks"
SEED_RES="$(curl -sfX POST "http://localhost:8080/dev/seed?many=$SEED_COUNT")" \
  || fail "seed POST failed"

# Extract aliceCookie without depending on jq (portable to bare zsh/bash).
ALICE_COOKIE="$(printf '%s' "$SEED_RES" | sed -n 's/.*"aliceCookie":"\([^"]*\)".*/\1/p')"
P_ID="$(printf '%s' "$SEED_RES" | sed -n 's/.*"pId":"\([^"]*\)".*/\1/p')"

# ── 5. Flush web ISR cache ───────────────────────────────────────────────────
if ! $NO_FLUSH; then
  step "flushing web .next/cache + restarting web"
  "${COMPOSE[@]}" exec -T web sh -c 'rm -rf .next/cache' \
    && "${COMPOSE[@]}" restart web >/dev/null
  wait_for "web (post-restart)" "curl -sf -o /dev/null http://localhost:3000"
fi

# ── 6. Summary ───────────────────────────────────────────────────────────────
echo
ok "stack ready"
echo "  http://localhost:3000     web"
echo "  http://localhost:8080     api"
echo "  http://localhost:9001     minio console (minioadmin/minioadmin)"
echo
ok "seeded ${SEED_COUNT} bulk artworks (+ public P, private Q)"
[[ -n "$P_ID" ]] && echo "  public artwork P id: $P_ID"
echo
echo "to sign in as alice, paste this into devtools › application › cookies:"
echo "  ${C_DIM}name:${C_RESET}  auth"
echo "  ${C_DIM}value:${C_RESET} ${ALICE_COOKIE#auth=}"
echo "  ${C_DIM}path:${C_RESET}  /"
