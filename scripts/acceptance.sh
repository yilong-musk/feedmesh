#!/usr/bin/env bash
# Acceptance run for the weibo-rss-pro card. Prints labelled evidence; never aborts.
set -u
export PATH=/home/ubuntu/.local/bin:$PATH
export WEIBO_RSS_CACHE=/home/ubuntu/.cache/weibo-rss-pro

hr() { printf '\n----- %s -----\n' "$1"; }

hr "1. build + vet + test"
cd /home/ubuntu/weibo-rss-pro
export PATH=/home/ubuntu/.hermes/tools/go-1.27.3-linux-x64/bin:$PATH
go build ./... && echo "go build: ok"
go vet ./... && echo "go vet: clean"
go test ./... 2>&1 | tail -8

hr "2a. stdout purity: fetch --json | jq (logs killed)"
weibo-rss-pro fetch 2803301701 --refresh --json 2>/dev/null > /tmp/pure.json
echo "stdout lines: $(wc -l < /tmp/pure.json)"
jq -e . /tmp/pure.json > /dev/null && echo "jq parse: OK" || echo "jq parse: FAILED"
jq -c '{ok,uid,screen_name,count}' /tmp/pure.json

hr "2b. three accounts, >=8 items each"
for u in 2803301701 2656274875 3937348351; do
  weibo-rss-pro fetch "$u" --refresh --json 2>/dev/null > /tmp/f_$u.json
  jq -r --arg u "$u" '"\(.uid) | \(.screen_name) | items=\(.items|length) | first=\(.items[0].bid) | cached=\(.cached)"' /tmp/f_$u.json
done

hr "3. doctor --json"
weibo-rss-pro doctor --json 2>/dev/null | jq -c .
echo "doctor exit: $?"

hr "4. HTTP endpoints (systemd unit on :3000)"
echo "-- /healthz"; curl -s http://127.0.0.1:3000/healthz | jq -c .
echo "-- /rss/user/2803301701"
curl -s -D /tmp/rss.h -o /tmp/rss.body http://127.0.0.1:3000/rss/user/2803301701
grep -i -E '^(HTTP|content-type)' /tmp/rss.h | tr -d '\r'
echo "bytes=$(wc -c < /tmp/rss.body) items=$(grep -c '<item>' /tmp/rss.body)"
head -c 200 /tmp/rss.body | head -3
echo "-- /api/v1/feed?uid=2803301701&limit=3"
curl -s 'http://127.0.0.1:3000/api/v1/feed?uid=2803301701&limit=3' | jq -c '{uid,screen_name,cached,n:(.items|length),ids:[.items[].bid]}'

hr "5. exit codes"
weibo-rss-pro fetch 9999999999999999 --json >/tmp/e1.json 2>/dev/null; echo "bad uid   -> exit=$?  $(cat /tmp/e1.json)"
weibo-rss-pro fetch 2803301701 --timeout 1ms --refresh --json >/tmp/e2.json 2>/dev/null; echo "timeout   -> exit=$?  $(cat /tmp/e2.json)"
weibo-rss-pro fetch --json >/tmp/e3.json 2>/dev/null; echo "usage     -> exit=$?  $(cat /tmp/e3.json)"

hr "6. systemd"
echo "is-active: $(systemctl --user is-active weibo-rss-pro)"
systemctl --user restart weibo-rss-pro
sleep 2
echo "after restart: $(systemctl --user is-active weibo-rss-pro)"
curl -s http://127.0.0.1:3000/healthz | jq -c '{status,version,uptime_s}'
systemctl --user show weibo-rss-pro -p MemoryCurrent --value | awk '{printf "RSS after restart: %.1f MB\n", $1/1048576}'

hr "7. cache dry-run (nothing deleted)"
weibo-rss-pro cache stat --json | jq -c .
weibo-rss-pro cache purge --all --dry-run --json | jq -c .
weibo-rss-pro cache stat --json | jq -c '{entries,bytes}'
