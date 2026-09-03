#!/bin/sh

set -u

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
installer=$repo_root/install.sh
original_path=${PATH:-/usr/bin:/bin}
test_root=$(mktemp -d "${TMPDIR:-/tmp}/devdash-install-test.XXXXXX")
passed=0

cleanup() {
	rm -rf "$test_root"
}
trap cleanup 0 HUP INT TERM

fail() {
	name=$1
	output_file=$2
	message=$3
	printf 'FAIL: %s: %s\n' "$name" "$message" >&2
	if [ -f "$output_file" ]; then
		printf '%s\n' '--- installer output ---' >&2
		cat "$output_file" >&2
	fi
	exit 1
}

pass() {
	passed=$((passed + 1))
	printf 'ok %d - %s\n' "$passed" "$1"
}

assert_contains() {
	name=$1
	output_file=$2
	needle=$3
	case "$(cat "$output_file")" in
		*"$needle"*)
			;;
		*)
			fail "$name" "$output_file" "expected output to contain: $needle"
			;;
	esac
}

assert_unchanged() {
	name=$1
	output_file=$2
	installed=$3
	if [ "$(cat "$installed")" != original ]; then
		fail "$name" "$output_file" "existing installation was modified"
	fi
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

write_checksums() {
	directory=$1
	: >"$directory/checksums.txt"
	for archive in "$directory"/devdash_*.tar.gz; do
		printf '%s  %s\n' "$(sha256_file "$archive")" "${archive##*/}" >>"$directory/checksums.txt"
	done
}

create_archive() {
	archive=$1
	identity=$2
	help_result=$3
	build_dir=$test_root/archive-build
	rm -rf "$build_dir"
	mkdir -p "$build_dir"
	cat >"$build_dir/devdash" <<EOF
#!/bin/sh
if [ "\${1:-}" = "--help" ]; then
	printf 'Usage: devdash\n'
	exit $help_result
fi
printf '%s\n' '$identity'
EOF
	chmod 0755 "$build_dir/devdash"
	tar -czf "$archive" -C "$build_dir" devdash
}

create_release_tree() {
	root=$1
	path_fragment=$2
	label=$3
	download_dir=$root/$path_fragment
	mkdir -p "$download_dir"
	create_archive "$download_dir/devdash_darwin_amd64.tar.gz" "$label-darwin-amd64" 0
	create_archive "$download_dir/devdash_darwin_arm64.tar.gz" "$label-darwin-arm64" 0
	create_archive "$download_dir/devdash_linux_amd64.tar.gz" "$label-linux-amd64" 0
	create_archive "$download_dir/devdash_linux_arm64.tar.gz" "$label-linux-arm64" 0
	write_checksums "$download_dir"
}

clone_release() {
	destination=$1
	mkdir -p "${destination%/*}"
	cp -R "$release_root" "$destination"
}

fake_bin=$test_root/fake-bin
mkdir -p "$fake_bin"
cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
	-s) printf '%s\n' "$FAKE_UNAME_S" ;;
	-m) printf '%s\n' "$FAKE_UNAME_M" ;;
	*) exit 1 ;;
esac
EOF
chmod 0755 "$fake_bin/uname"

release_root=$test_root/releases
create_release_tree "$release_root" latest/download latest
create_release_tree "$release_root" download/v0.1.0 version

check_mapping() {
	name=$1
	uname_s=$2
	uname_m=$3
	expected=$4
	case_root=$test_root/$name
	destination=$case_root/bin
	output=$case_root/output
	mkdir -p "$case_root"
	if ! FAKE_UNAME_S=$uname_s FAKE_UNAME_M=$uname_m \
		DEVDASH_INSTALL_DIR=$destination \
		DEVDASH_RELEASE_BASE_URL=file://$release_root \
		HOME=$case_root/home PATH=$fake_bin:$original_path \
		/bin/sh "$installer" >"$output" 2>&1; then
		fail "$name" "$output" "installer returned nonzero"
	fi
	destination_abs=$(CDPATH= cd "$destination" && pwd -P)
	if [ ! -x "$destination/devdash" ]; then
		fail "$name" "$output" "installed binary is not executable"
	fi
	if [ "$("$destination/devdash")" != "$expected" ]; then
		fail "$name" "$output" "installer selected the wrong asset"
	fi
	assert_contains "$name" "$output" "Devdash installed to $destination_abs/devdash"
	assert_contains "$name" "$output" "export PATH=\"$destination_abs:\$PATH\""
	pass "$name"
}

check_mapping darwin-amd64 Darwin x86_64 latest-darwin-amd64
check_mapping darwin-arm64 Darwin arm64 latest-darwin-arm64
check_mapping linux-amd64 Linux x86_64 latest-linux-amd64
check_mapping linux-arm64 Linux aarch64 latest-linux-arm64

name=version-and-default-home
case_root=$test_root/$name
home=$case_root/home
output=$case_root/output
mkdir -p "$home"
if ! (unset DEVDASH_INSTALL_DIR; \
	FAKE_UNAME_S=Linux FAKE_UNAME_M=amd64 \
	DEVDASH_VERSION=v0.1.0 DEVDASH_RELEASE_BASE_URL=file://$release_root \
	HOME=$home PATH=$fake_bin:$original_path \
	/bin/sh "$installer" >"$output" 2>&1); then
	fail "$name" "$output" "installer returned nonzero"
fi
if [ "$("$home/.local/bin/devdash")" != version-linux-amd64 ]; then
	fail "$name" "$output" "versioned release or default HOME destination was not used"
fi
pass "$name"

name=unsupported-os
case_root=$test_root/$name
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=FreeBSD FAKE_UNAME_M=x86_64 DEVDASH_INSTALL_DIR=$case_root/bin \
	PATH=$fake_bin:$original_path /bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: unsupported operating system: FreeBSD"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=unsupported-architecture
case_root=$test_root/$name
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=Linux FAKE_UNAME_M=riscv64 DEVDASH_INSTALL_DIR=$case_root/bin \
	PATH=$fake_bin:$original_path /bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: unsupported architecture: riscv64"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=checksum-mismatch
bad_release=$test_root/$name/releases
clone_release "$bad_release"
printf 'corruption' >>"$bad_release/latest/download/devdash_linux_amd64.tar.gz"
case_root=$test_root/$name/case
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 DEVDASH_INSTALL_DIR=$case_root/bin \
	DEVDASH_RELEASE_BASE_URL=file://$bad_release PATH=$fake_bin:$original_path \
	/bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: checksum verification failed for devdash_linux_amd64.tar.gz"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=missing-checksum-entry
bad_release=$test_root/$name/releases
clone_release "$bad_release"
checksums=$bad_release/latest/download/checksums.txt
awk '$2 != "devdash_linux_amd64.tar.gz"' "$checksums" >"$checksums.new"
mv "$checksums.new" "$checksums"
case_root=$test_root/$name/case
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 DEVDASH_INSTALL_DIR=$case_root/bin \
	DEVDASH_RELEASE_BASE_URL=file://$bad_release PATH=$fake_bin:$original_path \
	/bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: checksum entry not found for devdash_linux_amd64.tar.gz"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=missing-release-asset
bad_release=$test_root/$name/releases
clone_release "$bad_release"
rm "$bad_release/latest/download/devdash_linux_amd64.tar.gz"
case_root=$test_root/$name/case
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 DEVDASH_INSTALL_DIR=$case_root/bin \
	DEVDASH_RELEASE_BASE_URL=file://$bad_release PATH=$fake_bin:$original_path \
	/bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: failed to download"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=failed-executable-check
bad_release=$test_root/$name/releases
clone_release "$bad_release"
download_dir=$bad_release/latest/download
create_archive "$download_dir/devdash_linux_amd64.tar.gz" bad-executable 1
write_checksums "$download_dir"
case_root=$test_root/$name/case
mkdir -p "$case_root/bin"
printf 'original' >"$case_root/bin/devdash"
output=$case_root/output
if FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 DEVDASH_INSTALL_DIR=$case_root/bin \
	DEVDASH_RELEASE_BASE_URL=file://$bad_release PATH=$fake_bin:$original_path \
	/bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: downloaded devdash failed its executable check"
assert_unchanged "$name" "$output" "$case_root/bin/devdash"
pass "$name"

name=missing-checksum-command
limited_bin=$test_root/$name/bin
mkdir -p "$limited_bin"
for command_name in curl tar awk mktemp chmod cp mv mkdir rm uname; do
	ln -s "$(command -v "$command_name")" "$limited_bin/$command_name"
done
output=$test_root/$name/output
if PATH=$limited_bin DEVDASH_INSTALL_DIR=$test_root/$name/install \
	/bin/sh "$installer" >"$output" 2>&1; then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: required command not found: sha256sum or shasum"
pass "$name"

name=missing-home
output=$test_root/$name/output
mkdir -p "$test_root/$name"
if (unset HOME DEVDASH_INSTALL_DIR; PATH=$original_path /bin/sh "$installer" >"$output" 2>&1); then
	fail "$name" "$output" "installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" "error: HOME is not set; set DEVDASH_INSTALL_DIR explicitly"
pass "$name"

printf 'installer tests: %d passed\n' "$passed"
