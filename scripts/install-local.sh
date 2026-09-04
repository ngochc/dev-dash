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

for command_name in mktemp chmod cp mv mkdir rm; do
	require_command "$command_name"
done

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)

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

install_tmp=
cleanup() {
	status=$?
	trap - 0 HUP INT TERM
	if [ -n "$install_tmp" ]; then
		rm -f "$install_tmp"
	fi
	exit "$status"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

sh "$repo_root/scripts/build.sh"
candidate=$repo_root/bin/devdash
if ! "$candidate" --help >/dev/null 2>&1; then
	error "locally built devdash failed its executable check"
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
