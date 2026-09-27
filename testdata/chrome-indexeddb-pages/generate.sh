#!/usr/bin/env bash
# Regenerates ../chrome-indexeddb.leveldb: a real Chrome IndexedDB holding
# only the synthetic data in these pages. Three browser sessions produce a
# snappy-compressed table (bulk.html overflows the write buffer) plus a
# journal with a later insert and a deletion (second.html).
#
# With arguments, writes another directory from other pages, e.g. a Teams
# cache format sample:
#   ./generate.sh ../teams-formats/2026-09.leveldb teams-2026-09.html
set -euo pipefail

pages_dir="$(cd "$(dirname "$0")" && pwd)"
output_dir="$pages_dir/${1:-../chrome-indexeddb.leveldb}"
pages=("${@:2}")
[ ${#pages[@]} -gt 0 ] || pages=(index.html bulk.html second.html)
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

for page in "${pages[@]}"; do
	google-chrome-stable --headless=new --no-first-run --disable-gpu \
		--user-data-dir="$profile" "http://127.0.0.1:$port/$page" >/dev/null 2>&1 &
	chrome_pid=$!
	sleep 10
	# SIGTERM lets Chrome close LevelDB cleanly; --dump-dom exits too early.
	kill -TERM "$chrome_pid" 2>/dev/null || true
	sleep 4
done

rm -rf "$output_dir" && mkdir -p "$output_dir"
# A small database may have only the journal, no .ldb table yet.
shopt -s nullglob
cp "$profile"/Default/IndexedDB/*.indexeddb.leveldb/{*.log,*.ldb,CURRENT,MANIFEST-*} "$output_dir"/
ls -la "$output_dir"
