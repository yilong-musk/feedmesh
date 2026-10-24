#!/usr/bin/env bash
# Acceptance run for the feedmesh card. Prints labelled evidence; never aborts.
set -u
set -o pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${FEEDMESH_BIN:-feedmesh}"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/feedmesh-acceptance.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
chmod 700 "$TMP"
export FEEDMESH_CACHE="${FEEDMESH_CACHE:-$TMP/cache}"
ACCOUNTS="${FEEDMESH_ACCOUNTS:-2803301701 2656274875 3937348351}"

AUTH=()
if [ -n "${FEEDMESH_TOKEN:-}" ]; then
  AUTH=(-H "Authorization: Bearer ${FEEDMESH_TOKEN}")
fi

curl_auth() {
  if [ "${#AUTH[@]}" -gt 0 ]; then
    curl -s "${AUTH[@]}" "$@"
  else
    curl -s "$@"
  fi
}

hr() { printf '\n----- %s -----\n' "$1"; }

hr "1. build + vet + test"
cd "$ROOT" || exit 1
go build ./... && echo "go build: ok"
go vet ./... && echo "go vet: clean"
go test ./... >"$TMP/test.log" 2>&1
test_status=$?
tail -8 "$TMP/test.log"
echo "go test exit: $test_status"

hr "2a. stdout purity: fetch --json | jq (logs killed)"
"$BIN" fetch 2803301701 --refresh --json 2>/dev/null > "$TMP/pure.json"
echo "stdout lines: $(wc -l < "$TMP/pure.json")"
jq -e . "$TMP/pure.json" > /dev/null && echo "jq parse: OK" || echo "jq parse: FAILED"
jq -c '{ok,uid,screen_name,count}' "$TMP/pure.json"

hr "2b. accounts, >=8 items each"
for u in $ACCOUNTS; do
  "$BIN" fetch "$u" --refresh --json 2>/dev/null > "$TMP/f_$u.json"
  jq -r --arg u "$u" '"\(.uid) | \(.screen_name) | items=\(.items|length) | first=\(.items[0].bid) | cached=\(.cached)"' "$TMP/f_$u.json"
done

hr "3. doctor --json"
"$BIN" doctor --json 2>/dev/null | jq -c .
echo "doctor exit: $?"

hr "4. HTTP endpoints (systemd unit on :3000)"
echo "-- /healthz"; curl_auth http://127.0.0.1:3000/healthz | jq -c .
echo "-- /rss/user/2803301701"
curl_auth -D "$TMP/rss.h" -o "$TMP/rss.body" http://127.0.0.1:3000/rss/user/2803301701
grep -i -E '^(HTTP|content-type)' "$TMP/rss.h" | tr -d '\r'
echo "bytes=$(wc -c < "$TMP/rss.body") items=$(grep -c '<item>' "$TMP/rss.body")"
head -c 200 "$TMP/rss.body" | head -3
echo "-- /api/v1/feed?uid=2803301701&limit=3"
curl_auth 'http://127.0.0.1:3000/api/v1/feed?uid=2803301701&limit=3' | jq -c '{uid,screen_name,cached,n:(.items|length),ids:[.items[].bid]}'

hr "5. exit codes"
"$BIN" fetch 9999999999999999 --json >"$TMP/e1.json" 2>/dev/null; echo "bad uid   -> exit=$?  $(cat "$TMP/e1.json")"
"$BIN" fetch 2803301701 --timeout 1ms --refresh --json >"$TMP/e2.json" 2>/dev/null; echo "timeout   -> exit=$?  $(cat "$TMP/e2.json")"
"$BIN" fetch --json >"$TMP/e3.json" 2>/dev/null; echo "usage     -> exit=$?  $(cat "$TMP/e3.json")"

hr "6. systemd"
echo "is-active: $(systemctl --user is-active feedmesh)"
systemctl --user restart feedmesh
sleep 2
echo "after restart: $(systemctl --user is-active feedmesh)"
curl_auth http://127.0.0.1:3000/healthz | jq -c '{status,version,uptime_s}'
systemctl --user show feedmesh -p MemoryCurrent --value | awk '{printf "RSS after restart: %.1f MB\n", $1/1048576}'

hr "7. cache dry-run (nothing deleted)"
"$BIN" cache stat --json | jq -c .
"$BIN" cache purge --all --dry-run --json | jq -c .
"$BIN" cache stat --json | jq -c '{entries,bytes}'
