#!/bin/sh
# Runs all fuzz targets, one after the other, with 4 workers (5 test processes at most).
# Stops at the first crash; the failing input is written to testdata/fuzz/ of the package
# and runs with every `go test` afterwards.
#
# Usage: tools/fuzz.sh [time per target, default 5m]
set -e
cd "$(dirname "$0")/.."

fuzztime=${1:-5m}

for target in ./pkg/eml/:FuzzEnron ./pkg/eml/:FuzzANSI ./pkg/eml/:FuzzPrivate \
	./pkg/:FuzzRTFDecode ./pkg/:FuzzConvertRTF ./pkg/:FuzzParseNameToIDMap; do
	package=${target%%:*}
	name=${target#*:}
	echo "== $name ($fuzztime)"
	go test "$package" -run '^$' -parallel 4 -fuzz "^$name\$" -fuzztime "$fuzztime"
done
