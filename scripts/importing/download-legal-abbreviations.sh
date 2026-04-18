#!/usr/bin/env bash

# This script downloads the search results from the Cardiff Index to Legal
# Abbreviations (https://legalabbrevs.cardiff.ac.uk/) for entries whose
# jurisdiction is the United Kingdom or the United States. The result set is
# paginated across 76 pages; each page is saved as out/legal-abbreviations/
# page-NNN.html. Existing files are skipped so the script is resumable.
#
# Requires curl-impersonate (https://github.com/lwthiker/curl-impersonate)
# because the source server rejects requests from non-browser HTTP clients
# based on their TLS/HTTP fingerprint.

set -euo pipefail

OUT_DIR="out/legal-abbreviations"
PAGES=76

mkdir -p "$OUT_DIR"

for n in $(seq 1 "$PAGES"); do
    out=$(printf "%s/page-%03d.html" "$OUT_DIR" "$n")
    if [ -s "$out" ]; then
        continue
    fi
    url="https://legalabbrevs.cardiff.ac.uk/?form=search-en&paged=${n}&ilaw_jurisdiction%5B%5D=united-kingdom&ilaw_jurisdiction%5B%5D=united-states"
    curl_chrome116 -sSL --fail --retry 3 --retry-delay 5 -o "$out" "$url" \
        && echo "ok $out" \
        || { echo "FAIL $out" >&2; rm -f "$out"; }
    sleep 2
done
