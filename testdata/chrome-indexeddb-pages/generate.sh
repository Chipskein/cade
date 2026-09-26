#!/usr/bin/env bash
# Regenerates ../chrome-indexeddb.leveldb: a real Chrome IndexedDB holding
# only the synthetic data in these pages. Three browser sessions produce a
# snappy-compressed table (bulk.html overflows the write buffer) plus a
# journal with a later insert and a deletion (second.html).
set -euo pipefail

pages_dir="$(cd "$(dirname "$0")" && pwd)"
output_dir="$pages_dir/../chrome-indexeddb.leveldb"
port=8765
profile="$(mktemp -d)"
trap 'kill "$server_pid" 2>/dev/null || true; rm -rf "$profile"' EXIT

if ss -ltn | grep -q ":$port "; then
	echo "port $port is busy; a stale server would serve old pages" >&2
	exit 1
fi
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$pages_dir" >/dev/null 2>&1 &
server_pid=$!
sleep 1

for page in index.html bulk.html second.html; do
	google-chrome-stable --headless=new --no-first-run --disable-gpu \
		--user-data-dir="$profile" "http://127.0.0.1:$port/$page" >/dev/null 2>&1 &
	chrome_pid=$!
	sleep 10
	# SIGTERM lets Chrome close LevelDB cleanly; --dump-dom exits too early.
	kill -TERM "$chrome_pid" 2>/dev/null || true
	sleep 4
done

rm -rf "$output_dir" && mkdir -p "$output_dir"
cp "$profile"/Default/IndexedDB/*.indexeddb.leveldb/{*.log,*.ldb,CURRENT,MANIFEST-*} "$output_dir"/
ls -la "$output_dir"
