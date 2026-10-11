#!/usr/bin/env bash
# Regenerates ../firefox-indexeddb: a real Firefox (Floorp) IndexedDB
# holding only the synthetic data in index.html. Only the *.sqlite files
# are kept: the reader never opens the .files directory, so the Blob and
# the external clone it holds stay out of the repository.
#
# Uses floorp, or firefox when floorp is missing.
set -euo pipefail

pages_dir="$(cd "$(dirname "$0")" && pwd)"
output_dir="$pages_dir/../firefox-indexeddb"
port=8766
profile="$(mktemp -d)"
browser="$(command -v floorp || command -v firefox)"
trap 'kill "$server_pid" 2>/dev/null || true; rm -rf "$profile"' EXIT

if ss -ltn | grep -q ":$port "; then
	echo "port $port is busy; a stale server would serve old pages" >&2
	exit 1
fi
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$pages_dir" >/dev/null 2>&1 &
server_pid=$!
sleep 1

"$browser" --headless --no-remote --profile "$profile" "http://127.0.0.1:$port/index.html" >/dev/null 2>&1 &
browser_pid=$!
sleep 15
# SIGTERM makes Firefox shut down cleanly and checkpoint the WAL.
kill -TERM "$browser_pid" 2>/dev/null || true
wait "$browser_pid" 2>/dev/null || true

rm -rf "$output_dir" && mkdir -p "$output_dir"
cp "$profile"/storage/default/http+++127.0.0.1+"$port"/idb/*.sqlite "$output_dir"/
ls -la "$output_dir"
