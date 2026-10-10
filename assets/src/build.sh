#!/bin/sh
# Rebuilds assets/banner and assets/nav. Text is converted to paths because
# SVGs shown through <img> cannot load fonts. Needs python3, inkscape, scour.
set -e
root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$root/.tmp/src"
mkdir -p "$work"
OUT="$work" python3 "$root/assets/src/gen.py"
for f in "$work"/src-*.svg; do
  name=$(basename "$f" .svg); name=${name#src-}
  case $name in banner-*) dest=banner ;; *) dest=nav ;; esac
  inkscape --export-text-to-path --export-plain-svg --export-filename="$work/conv-$name.svg" "$f" 2>/dev/null
  scour -i "$work/conv-$name.svg" -o "$root/assets/$dest/$name.svg" --set-precision=5 --enable-id-stripping --shorten-ids >/dev/null
done
