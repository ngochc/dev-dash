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

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
	printf 'usage: scripts/publish-release.sh <commit>\n' >&2
	exit 1
fi
commit=$1

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
cd "$repo_root"

for command_name in go git gh; do
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

if tag_commit=$(git rev-list -n 1 "refs/tags/$version" 2>/dev/null); then
	:
else
	tag_commit=
fi
if gh release view "$version" >/dev/null 2>&1; then
	release_exists=true
else
	release_exists=false
fi

release_note="> This release was automatically published by the Release workflow from commit $commit."

create_release() {
	gh release create "$version" \
		dist/devdash_darwin_amd64.tar.gz \
		dist/devdash_darwin_arm64.tar.gz \
		dist/devdash_linux_amd64.tar.gz \
		dist/devdash_linux_arm64.tar.gz \
		dist/checksums.txt \
		--verify-tag --generate-notes \
		--notes "$release_note"
}

upload_release() {
	gh release upload "$version" \
		dist/devdash_darwin_amd64.tar.gz \
		dist/devdash_darwin_arm64.tar.gz \
		dist/devdash_linux_amd64.tar.gz \
		dist/devdash_linux_arm64.tar.gz \
		dist/checksums.txt \
		--clobber
}

if [ -z "$tag_commit" ]; then
	sh scripts/release.sh
	git tag "$version" "$commit"
	git push origin "refs/tags/$version"
	create_release
	exit 0
fi

if [ "$tag_commit" = "$commit" ]; then
	sh scripts/release.sh
	if [ "$release_exists" = true ]; then
		upload_release
	else
		create_release
	fi
	exit 0
fi

if [ "$release_exists" = true ]; then
	printf 'Release %s already exists from %s; nothing to publish.\n' "$version" "$tag_commit"
	exit 0
fi

error "tag $version points to $tag_commit, not $commit, and has no release"
