#!/bin/sh

set -eu

error() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
cd "$repo_root"

coverage_file=$(mktemp "${TMPDIR:-/tmp}/devdash-coverage.XXXXXX")
cleanup() {
	status=$?
	trap - 0 HUP INT TERM
	rm -f "$coverage_file"
	exit "$status"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

for script in \
	install.sh \
	scripts/build.sh \
	scripts/install-local.sh \
	scripts/test.sh \
	scripts/release.sh \
	test/install_test.sh \
	test/tooling_test.sh
do
	sh -n "$script"
done

sh test/install_test.sh
sh test/tooling_test.sh
go test ./... -coverprofile="$coverage_file"

coverage_report=$(go tool cover -func="$coverage_file")
printf '%s\n' "$coverage_report"
coverage=$(printf '%s\n' "$coverage_report" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
if [ -z "$coverage" ]; then
	error "total coverage was not reported"
fi
if ! awk -v coverage="$coverage" 'BEGIN { exit !(coverage > 80.0) }'; then
	error "total coverage must be greater than 80.0% (got $coverage%)"
fi
