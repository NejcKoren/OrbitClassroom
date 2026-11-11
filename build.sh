#!/usr/bin/env bash
# Builds Orbit Classroom for Windows (x64 and ARM64) from Linux/macOS/WSL.
#   ./build.sh            -> dist/OrbitClassroom-<version>-windows-x64.zip (+ arm64)
# Requirements: Go 1.24+, go-winres (go install github.com/tc-hib/go-winres@latest)
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${VERSION:-1.0.0}"
WINRES="${WINRES:-$(go env GOPATH)/bin/go-winres}"
DIST=dist
LDFLAGS="-s -w -H windowsgui -X orbit/internal/proto.Version=${VERSION}"

echo "== tests"
go vet ./internal/proto/ ./internal/master/ ./internal/agent/
go test ./...
python3 tools/checki18n/check.py

echo "== icons & resources"
rm -rf build && mkdir -p build/icons
go run ./tools/genicon build/icons
mkres() { # name description exe dir
  local v4="${VERSION}.0"
  cat > "build/$1.json" <<JSON
{
  "RT_GROUP_ICON": { "APP": { "0000": ["icons/icon256.png","icons/icon128.png","icons/icon64.png","icons/icon48.png","icons/icon40.png","icons/icon32.png","icons/icon24.png","icons/icon20.png","icons/icon16.png"] } },
  "RT_MANIFEST": { "#1": { "0409": {
    "identity": { "name": "OrbitClassroom.$1", "version": "$v4" },
    "description": "$2", "minimum-os": "win10", "execution-level": "as invoker",
    "ui-access": false, "auto-elevate": false, "dpi-awareness": "per monitor v2",
    "disable-theming": false, "disable-window-filtering": false,
    "high-resolution-scrolling-aware": true, "ultra-high-resolution-scrolling-aware": false,
    "long-path-aware": true, "printer-driver-isolation": false, "gdi-scaling": false,
    "segment-heap": false, "use-common-controls-v6": true } } },
  "RT_VERSION": { "#1": { "0000": {
    "fixed": { "file_version": "$v4", "product_version": "$v4" },
    "info": { "0409": {
      "CompanyName": "Orbit Classroom", "FileDescription": "$2", "FileVersion": "$VERSION",
      "InternalName": "$3", "LegalCopyright": "Orbit Classroom", "OriginalFilename": "$3",
      "ProductName": "Orbit Classroom", "ProductVersion": "$VERSION" } } } } }
}
JSON
  (cd build && "$WINRES" make --in "$1.json" --arch amd64,arm64 --out "../$4/rsrc")
}
mkres Master "Orbit Master - classroom management (teacher)" OrbitMaster.exe cmd/master
mkres Agent "Orbit Agent - classroom management (student)" OrbitAgent.exe cmd/agent

for ARCH in amd64 arm64; do
  NAME=x64; [ "$ARCH" = arm64 ] && NAME=arm64
  OUT="$DIST/OrbitClassroom-$VERSION-windows-$NAME"
  echo "== windows/$ARCH -> $OUT"
  rm -rf "$OUT" && mkdir -p "$OUT/lang"
  GOOS=windows GOARCH=$ARCH CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/OrbitAgent.exe" ./cmd/agent
  cp "$OUT/OrbitAgent.exe" cmd/master/embedded/OrbitAgent.exe   # master can hand out the agent
  GOOS=windows GOARCH=$ARCH CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/OrbitMaster.exe" ./cmd/master
  rm -f cmd/master/embedded/OrbitAgent.exe
  # documentation: the same files the program shows under "Help & guide"
  sed 's/$/\r/' internal/master/web/docs/en-simple.md > "$OUT/START-HERE-EN.txt"
  sed 's/$/\r/' internal/master/web/docs/sl-simple.md > "$OUT/ZACNI-TUKAJ-SL.txt"
  sed 's/$/\r/' internal/master/web/docs/en.md > "$OUT/GUIDE_EN.txt"
  sed 's/$/\r/' internal/master/web/docs/sl.md > "$OUT/NAVODILA_SL.txt"
  sed 's/$/\r/' docs/README-FIRST.txt > "$OUT/README-FIRST.txt"
  sed 's/$/\r/' docs/lang-README.txt > "$OUT/lang/README.txt"
  (cd "$DIST" && rm -f "$(basename "$OUT").zip" && zip -qr "$(basename "$OUT").zip" "$(basename "$OUT")")
done

echo "== source package"
git ls-files >/dev/null 2>&1 && SRC_LIST=$(git ls-files) || SRC_LIST=$(find . -type f -not -path './dist/*' -not -path './build/*' -not -path './.git/*' -not -name '*.syso' | sed 's|^\./||')
rm -f "$DIST/OrbitClassroom-$VERSION-source.zip"
echo "$SRC_LIST" | zip -q "$DIST/OrbitClassroom-$VERSION-source.zip" -@
ls -la "$DIST"
