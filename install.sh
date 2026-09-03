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

for command_name in curl tar awk mktemp chmod cp mv mkdir rm uname; do
	require_command "$command_name"
done

if command -v sha256sum >/dev/null 2>&1; then
	checksum_command=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	checksum_command=shasum
else
	error "required command not found: sha256sum or shasum"
fi

if [ "${DEVDASH_INSTALL_DIR+x}" = x ]; then
	install_dir=$DEVDASH_INSTALL_DIR
else
	if [ -z "${HOME:-}" ]; then
		error "HOME is not set; set DEVDASH_INSTALL_DIR explicitly"
	fi
	install_dir=$HOME/.local/bin
fi

if [ -z "$install_dir" ]; then
	error "install directory is empty"
fi

if ! mkdir -p "$install_dir"; then
	error "failed to create install directory: $install_dir"
fi
if ! install_dir=$(CDPATH= cd "$install_dir" && pwd -P); then
	error "failed to resolve install directory: $install_dir"
fi

case "$(uname -s)" in
	Darwin)
		os=darwin
		;;
	Linux)
		os=linux
		;;
	*)
		error "unsupported operating system: $(uname -s)"
		;;
esac

case "$(uname -m)" in
	x86_64 | amd64)
		arch=amd64
		;;
	arm64 | aarch64)
		arch=arm64
		;;
	*)
		error "unsupported architecture: $(uname -m)"
		;;
esac

version=${DEVDASH_VERSION:-latest}
release_base=${DEVDASH_RELEASE_BASE_URL:-https://github.com/ngochc/dev-dash/releases}
if [ "$version" = latest ]; then
	download_base=$release_base/latest/download
else
	download_base=$release_base/download/$version
fi

asset=devdash_${os}_${arch}.tar.gz
download_dir=
install_tmp=

cleanup() {
	status=$?
	trap - 0 HUP INT TERM
	if [ -n "$install_tmp" ]; then
		rm -f "$install_tmp"
	fi
	if [ -n "$download_dir" ]; then
		rm -rf "$download_dir"
	fi
	exit "$status"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

if ! download_dir=$(mktemp -d "${TMPDIR:-/tmp}/devdash-install.XXXXXX"); then
	error "failed to create temporary directory"
fi
archive=$download_dir/$asset
checksums=$download_dir/checksums.txt

if ! curl -fsSL "$download_base/$asset" -o "$archive"; then
	error "failed to download $download_base/$asset"
fi
if ! curl -fsSL "$download_base/checksums.txt" -o "$checksums"; then
	error "failed to download $download_base/checksums.txt"
fi

if ! checksum_line=$(awk -v asset="$asset" '
	{
		name = $2
		sub(/^\*/, "", name)
		if (name == asset) {
			print $1 "  " asset
			found = 1
			exit
		}
	}
	END { if (!found) exit 1 }
' "$checksums"); then
	error "checksum entry not found for $asset"
fi

if [ "$checksum_command" = sha256sum ]; then
	if ! (cd "$download_dir" && printf '%s\n' "$checksum_line" | sha256sum -c - >/dev/null); then
		error "checksum verification failed for $asset"
	fi
else
	if ! (cd "$download_dir" && printf '%s\n' "$checksum_line" | shasum -a 256 -c - >/dev/null); then
		error "checksum verification failed for $asset"
	fi
fi

extract_dir=$download_dir/extract
if ! mkdir "$extract_dir"; then
	error "failed to create extraction directory"
fi
if ! tar -xzf "$archive" -C "$extract_dir" devdash; then
	error "failed to extract $asset"
fi
candidate=$extract_dir/devdash
if [ ! -f "$candidate" ] || [ ! -x "$candidate" ]; then
	error "release archive does not contain an executable devdash"
fi
if ! "$candidate" --help >/dev/null 2>&1; then
	error "downloaded devdash failed its executable check"
fi

if ! install_tmp=$(mktemp "$install_dir/.devdash.tmp.XXXXXX"); then
	error "failed to create temporary install file in $install_dir"
fi
if ! cp "$candidate" "$install_tmp"; then
	error "failed to copy devdash into $install_dir"
fi
if ! chmod 0755 "$install_tmp"; then
	error "failed to make devdash executable"
fi
if ! mv -f "$install_tmp" "$install_dir/devdash"; then
	error "failed to install devdash to $install_dir/devdash"
fi
install_tmp=

printf 'Devdash installed to %s/devdash\n' "$install_dir"
case ":${PATH:-}:" in
	*":$install_dir:"*)
		;;
	*)
		printf 'Add Devdash to your PATH:\n  export PATH="%s:$PATH"\n' "$install_dir"
		;;
esac
