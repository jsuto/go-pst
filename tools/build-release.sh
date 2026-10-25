#!/bin/sh
# Builds the pst2eml release archives into dist/<version>/: static binaries (no cgo) for Linux, Windows and
# macOS on amd64 and arm64, each archive with LICENSE.txt and README.md, and SHA256SUMS.
#
# Usage: tools/build-release.sh <version, e.g. v0.2.0>
set -eu
cd "$(dirname "$0")/.."

version=${1:?usage: tools/build-release.sh <version>}
output="dist/$version"

rm -rf "$output"
mkdir -p "$output"

for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64; do
	goos=${target%/*}
	goarch=${target#*/}
	name="pst2eml_${version}_${goos}_${goarch}"
	binary=pst2eml

	if [ "$goos" = windows ]; then
		binary=pst2eml.exe
	fi

	mkdir -p "$output/$name"
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "-s -w -X main.version=$version" \
		-o "$output/$name/$binary" ./cmd/pst2eml
	cp LICENSE.txt README.md "$output/$name/"

	if [ "$goos" = windows ]; then
		(cd "$output" && zip -qr "$name.zip" "$name")
	else
		tar -C "$output" -czf "$output/$name.tar.gz" "$name"
	fi

	rm -r "${output:?}/$name"
	echo "$output/$name"
done

(cd "$output" && shasum -a 256 ./*.tar.gz ./*.zip | sed 's| \./| |' > SHA256SUMS)
