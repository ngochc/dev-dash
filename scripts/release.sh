#!/bin/sh

set -eu

error() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		error "required command not found: $1"
	fi
}

if [ "$#" -ne 0 ]; then
	printf 'usage: scripts/release.sh\n' >&2
	exit 1
fi

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
cd "$repo_root"

for command_name in go git tar curl awk mktemp chmod cp mv mkdir rm uname; do
	require_command "$command_name"
done

reported_version=$(go run ./cmd/devdash version)
case "$reported_version" in
	"devdash v"*)
		version=${reported_version#devdash }
		;;
	*)
		error "application version must start with v (got $reported_version)"
		;;
esac
if ! git check-ref-format "refs/tags/$version" >/dev/null 2>&1; then
	error "application version is not a valid tag: $version"
fi

host_os=$(uname -s)
host_arch=$(uname -m)
case "$host_os" in
	Darwin | Linux)
		;;
	*)
		error "unsupported release host: $host_os/$host_arch"
		;;
esac
case "$host_arch" in
	x86_64 | amd64 | arm64 | aarch64)
		;;
	*)
		error "unsupported release host: $host_os/$host_arch"
		;;
esac

if command -v sha256sum >/dev/null 2>&1; then
	checksum_command=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	checksum_command=shasum
else
	error "required command not found: sha256sum or shasum"
fi

sh scripts/test.sh

release_root=
cleanup() {
	status=$?
	trap - 0 HUP INT TERM
	if [ -n "$release_root" ]; then
		rm -rf "$release_root"
	fi
	exit "$status"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

if ! release_root=$(mktemp -d "${TMPDIR:-/tmp}/devdash-release.XXXXXX"); then
	error "failed to create temporary release directory"
fi
staged_dist=$release_root/dist
mkdir -p "$staged_dist"

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
	goos=${target%/*}
	goarch=${target#*/}
	build_dir=$release_root/build/${goos}_${goarch}
	mkdir -p "$build_dir"
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
		go build -trimpath \
		-ldflags "-s -w" \
		-o "$build_dir/devdash" ./cmd/devdash
	tar -czf "$staged_dist/devdash_${goos}_${goarch}.tar.gz" -C "$build_dir" devdash
done

(
	cd "$staged_dist"
	if [ "$checksum_command" = sha256sum ]; then
		for archive in \
			devdash_darwin_amd64.tar.gz \
			devdash_darwin_arm64.tar.gz \
			devdash_linux_amd64.tar.gz \
			devdash_linux_arm64.tar.gz
		do
			sha256sum "$archive"
		done
	else
		for archive in \
			devdash_darwin_amd64.tar.gz \
			devdash_darwin_arm64.tar.gz \
			devdash_linux_amd64.tar.gz \
			devdash_linux_arm64.tar.gz
		do
			shasum -a 256 "$archive"
		done
	fi
) >"$staged_dist/checksums.txt"

fixture=$release_root/release/latest/download
mkdir -p "$fixture"
cp "$staged_dist"/devdash_*.tar.gz "$staged_dist/checksums.txt" "$fixture/"
install_dir=$release_root/install-bin
DEVDASH_RELEASE_BASE_URL=file://$release_root/release \
	DEVDASH_INSTALL_DIR=$install_dir \
	sh install.sh
if ! "$install_dir/devdash" --help >/dev/null 2>&1; then
	error "installed release failed its executable check"
fi
actual_version=$("$install_dir/devdash" version)
if [ "$actual_version" != "devdash $version" ]; then
	error "installed release version mismatch: expected devdash $version, got $actual_version"
fi

rm -rf "$repo_root/dist"
mv "$staged_dist" "$repo_root/dist"
printf 'Release artifacts written to %s/dist\n' "$repo_root"
