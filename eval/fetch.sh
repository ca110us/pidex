#!/bin/bash
# Downloads the eval photos listed in SET/sources.tsv from Wikimedia
# Commons. The photos are not kept in the repo because their licenses
# (recorded in sources.tsv) require attribution.
#   eval/fetch.sh [SET [WIDTH]]   (commons, appearance or mugs; default commons, 1280)
set -euo pipefail
cd "$(dirname "$0")"
SET=${1:-commons}
WIDTH=${2:-1280}
OUT=$SET/images
UA="PidexEval/0.1 (photo-classification test set; low volume)"

while IFS=$'\t' read -r path title _license; do
  dest="$OUT/$path"
  [ -f "$dest" ] && continue
  mkdir -p "$(dirname "$dest")"
  t=$(jq -rn --arg t "$title" '$t|@uri')
  thumb=$(curl -sf -A "$UA" "https://commons.wikimedia.org/w/api.php?action=query&format=json&prop=imageinfo&iiprop=url&iiurlwidth=$WIDTH&titles=$t" |
    jq -r '.query.pages[].imageinfo[0].thumburl // empty')
  if [ -z "$thumb" ]; then
    echo "not found: $title" >&2
    continue
  fi
  sleep 2
  curl -sf -A "$UA" "$thumb" -o "$dest.tmp" && sips -s format jpeg "$dest.tmp" --out "$dest" >/dev/null && rm -f "$dest.tmp"
  echo "$path"
done < "$SET/sources.tsv"
