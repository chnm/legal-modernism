#!/usr/bin/env bash

# This script downloads the search results from the Cardiff Index to Legal
# Abbreviations (https://legalabbrevs.cardiff.ac.uk/) for entries whose
# jurisdiction is the United Kingdom, the United States, or any US state.
# The result set is paginated across 172 pages; each page is saved as
# out/legal-abbreviations/page-NNN.html. Existing files are skipped so the
# script is resumable.
#
# Requires curl-impersonate (https://github.com/lwthiker/curl-impersonate)
# because the source server rejects requests from non-browser HTTP clients
# based on their TLS/HTTP fingerprint.

set -euo pipefail

OUT_DIR="out/legal-abbreviations"
PAGES=172

JURISDICTIONS=(
    england-ireland
    england-wales
    united-kingdom
    united-states
    united-states-alabama
    united-states-alaska
    united-states-arizona
    united-states-arkansas
    united-states-california
    united-states-colorado
    united-states-connecticut
    united-states-dakota
    united-states-delaware
    united-states-district-of-columbia
    united-states-florida
    united-states-georgia
    united-states-hawaii
    united-states-idaho
    united-states-illinois
    united-states-indiana
    united-states-iowa
    united-states-kansas
    united-states-kentucky
    united-states-louisiana
    united-states-maine
    united-states-maryland
    united-states-massachusetts
    united-states-michigan
    united-states-minnesota
    united-states-mississippi
    united-states-missouri
    united-states-montana
    united-states-navajo-nation
    united-states-nebraska
    united-states-nevada
    united-states-new-hampshire
    united-states-new-jersey
    united-states-new-mexico
    united-states-new-york
    united-states-north-carolina
    united-states-north-dakota
    united-states-ohio
    united-states-oklahoma
    united-states-oregon
    united-states-pennsylvania
    united-states-rhode-island
    united-states-south-carolina
    united-states-south-dakota
    united-states-tennessee
    united-states-texas
    united-states-utah
    united-states-vermont
    united-states-virginia
    united-states-washington
    united-states-west-virginia
    united-states-western-samoa
    united-states-wisconsin
    united-states-wyoming
)

juris_query=""
for j in "${JURISDICTIONS[@]}"; do
    juris_query+="&ilaw_jurisdiction%5B%5D=${j}"
done

mkdir -p "$OUT_DIR"

for n in $(seq 1 "$PAGES"); do
    out=$(printf "%s/page-%03d.html" "$OUT_DIR" "$n")
    if [ -s "$out" ]; then
        continue
    fi
    url="https://legalabbrevs.cardiff.ac.uk/?form=search-en&paged=${n}${juris_query}"
    curl_chrome116 -sSL --fail --retry 3 --retry-delay 5 -o "$out" "$url" \
        && echo "ok $out" \
        || { echo "FAIL $out" >&2; rm -f "$out"; }
    sleep 2
done
