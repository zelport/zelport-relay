#!/bin/sh
# Run preserved SDK white-box integration tests against this staged relay version.
set -eu
cd "$(dirname "$0")/.."
: "${ZELPORT_RELAY_VERSION:?run through the staging run.sh, or provide the published relay version}"
GO=${GO:-go}
export GOWORK=off
"$GO" mod download github.com/zelport/zelport-sdk-go
sdk_dir=$("$GO" list -m -f '{{.Dir}}' github.com/zelport/zelport-sdk-go)
if [ -z "$sdk_dir" ] || [ ! -f "$sdk_dir/go.mod" ] || [ ! -d "$sdk_dir/client" ]; then
  echo 'Cannot resolve SDK source directory; refusing integration copy' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
cp -R "$sdk_dir/." "$work/"
chmod -R u+w "$work"
for fixture in tests/sdk-client/*.go.txt; do
  cp "$fixture" "$work/client/$(basename "$fixture" .txt)"
done
cd "$work"
"$GO" mod edit -require="github.com/zelport/zelport-relay@$ZELPORT_RELAY_VERSION"
"$GO" mod tidy
"$GO" test -race ./client "$@"
