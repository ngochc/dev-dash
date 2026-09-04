#!/bin/sh

set -u

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
original_path=${PATH:-/usr/bin:/bin}
test_root=$(mktemp -d "${TMPDIR:-/tmp}/devdash-tooling-test.XXXXXX")
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
		printf '%s\n' '--- command output ---' >&2
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

assert_not_contains() {
	name=$1
	output_file=$2
	needle=$3
	case "$(cat "$output_file")" in
		*"$needle"*)
			fail "$name" "$output_file" "expected output not to contain: $needle"
			;;
		*)
			;;
	esac
}

assert_unchanged() {
	name=$1
	output_file=$2
	path=$3
	if [ ! -f "$path" ] || [ "$(cat "$path")" != original ]; then
		fail "$name" "$output_file" "existing content was modified"
	fi
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

fake_bin=$test_root/fake-bin
mkdir -p "$fake_bin"
cat >"$fake_bin/go" <<'EOF'
#!/bin/sh
set -u

printf 'CGO_ENABLED=%s|GOOS=%s|GOARCH=%s|%s\n' \
	"${CGO_ENABLED:-}" "${GOOS:-}" "${GOARCH:-}" "$*" >>"${FAKE_GO_LOG:-/dev/null}"
if [ -n "${PUBLISH_CALL_LOG:-}" ]; then
	printf 'go:%s\n' "$*" >>"$PUBLISH_CALL_LOG"
fi

if [ "${1:-}" = test ]; then
	for argument in "$@"; do
		case "$argument" in
			-coverprofile=*)
				coverage_file=${argument#*=}
				: >"$coverage_file"
				;;
		esac
	done
	exit "${FAKE_GO_TEST_RESULT:-0}"
fi

if [ "${1:-}" = tool ] && [ "${2:-}" = cover ]; then
	printf '%s\n' "${FAKE_COVERAGE_OUTPUT:-}"
	exit 0
fi

if [ "${1:-}" = run ] && [ "${2:-}" = ./cmd/devdash ] && [ "${3:-}" = version ] && [ "$#" -eq 3 ]; then
	printf 'devdash %s\n' "${FAKE_APP_VERSION:-v0.1.2}"
	exit 0
fi

if [ "${1:-}" != build ]; then
	exit 1
fi
if [ "${FAKE_GO_MODE:-success}" = fail ]; then
	exit 1
fi

output=
version=${FAKE_APP_VERSION:-v0.1.2}
shift
while [ "$#" -gt 0 ]; do
	argument=$1
	case "$argument" in
		-o)
			shift
			output=$1
			;;
	esac
	shift
done
if [ -z "$output" ]; then
	exit 1
fi
mkdir -p "${output%/*}"
help_result=0
if [ "${FAKE_GO_MODE:-success}" = bad-help ]; then
	help_result=1
fi
cat >"$output" <<EOF_BINARY
#!/bin/sh
if [ "\${1:-}" = "--help" ]; then
	exit $help_result
fi
if [ "\${1:-}" = "version" ]; then
	printf '%s\n' 'devdash $version'
	exit 0
fi
printf '%s\n' 'devdash $version'
EOF_BINARY
chmod 0755 "$output"
EOF
chmod 0755 "$fake_bin/go"

cat >"$fake_bin/git" <<'EOF'
#!/bin/sh
set -u

if [ -n "${PUBLISH_CALL_LOG:-}" ]; then
	printf 'git:%s\n' "$*" >>"$PUBLISH_CALL_LOG"
fi

case "${1:-}" in
	check-ref-format)
		exit "${FAKE_GIT_CHECK_RESULT:-0}"
		;;
	rev-list)
		if [ -z "${FAKE_TAG_COMMIT:-}" ]; then
			exit 1
		fi
		printf '%s\n' "$FAKE_TAG_COMMIT"
		;;
	tag | push)
		exit "${FAKE_GIT_MUTATION_RESULT:-0}"
		;;
	*)
		exit 1
		;;
esac
EOF
chmod 0755 "$fake_bin/git"

cat >"$fake_bin/gh" <<'EOF'
#!/bin/sh
set -u

if [ -n "${PUBLISH_CALL_LOG:-}" ]; then
	printf 'gh:%s\n' "$*" >>"$PUBLISH_CALL_LOG"
fi

if [ "${1:-}" = release ] && [ "${2:-}" = view ]; then
	if [ "${FAKE_RELEASE_EXISTS:-no}" = yes ]; then
		exit 0
	fi
	exit 1
fi
if [ "${1:-}" = release ] && { [ "${2:-}" = create ] || [ "${2:-}" = upload ]; }; then
	exit "${FAKE_GH_RESULT:-0}"
fi
exit 1
EOF
chmod 0755 "$fake_bin/gh"

cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
	-s) printf '%s\n' "${FAKE_UNAME_S:-Linux}" ;;
	-m) printf '%s\n' "${FAKE_UNAME_M:-x86_64}" ;;
	*) exit 1 ;;
esac
EOF
chmod 0755 "$fake_bin/uname"

name=make-dispatch
case_root=$test_root/$name
fixture=$case_root/repo
output=$case_root/output
calls=$case_root/calls
mkdir -p "$fixture/scripts"
cp "$repo_root/Makefile" "$fixture/Makefile"
for script in build.sh install-local.sh test.sh release.sh; do
	cat >"$fixture/scripts/$script" <<'EOF'
#!/bin/sh
printf '%s:%s\n' "${0##*/}" "$*" >>"$CALL_LOG"
EOF
	chmod 0755 "$fixture/scripts/$script"
done
if ! CALL_LOG=$calls make -s -C "$fixture" >"$output" 2>&1 ||
	! CALL_LOG=$calls make -s -C "$fixture" install >>"$output" 2>&1 ||
	! CALL_LOG=$calls make -s -C "$fixture" test >>"$output" 2>&1 ||
	! CALL_LOG=$calls make -s -C "$fixture" release >>"$output" 2>&1; then
	fail "$name" "$output" "Make target dispatch failed"
fi
expected_calls='build.sh:
install-local.sh:
test.sh:
release.sh:'
if [ "$(cat "$calls")" != "$expected_calls" ]; then
	fail "$name" "$output" "Make dispatched an unexpected script or argument"
fi
pass "$name"

name=build-from-non-root
case_root=$test_root/$name
fixture=$case_root/repo
output=$case_root/output
go_log=$case_root/go.log
mkdir -p "$fixture/scripts" "$case_root/work"
cp "$repo_root/scripts/build.sh" "$fixture/scripts/build.sh"
if ! (cd "$case_root/work" && FAKE_GO_LOG=$go_log PATH=$fake_bin:$original_path \
	/bin/sh "$fixture/scripts/build.sh" >"$output" 2>&1); then
	fail "$name" "$output" "build script returned nonzero"
fi
if [ ! -x "$fixture/bin/devdash" ]; then
	fail "$name" "$output" "build output is missing or not executable"
fi
assert_contains "$name" "$go_log" '|build -o bin/devdash ./cmd/devdash'
pass "$name"

setup_install_fixture() {
	install_fixture=$1/repo
	mkdir -p "$install_fixture/scripts"
	cp "$repo_root/scripts/build.sh" "$install_fixture/scripts/build.sh"
	cp "$repo_root/scripts/install-local.sh" "$install_fixture/scripts/install-local.sh"
}

name=install-explicit-destination
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
setup_install_fixture "$case_root"
destination=$case_root/install/bin
mkdir -p "$case_root/work"
if ! (cd "$case_root/work" && DEVDASH_INSTALL_DIR=$destination FAKE_GO_LOG=$go_log \
	PATH=$fake_bin:$original_path /bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1); then
	fail "$name" "$output" "local installer returned nonzero"
fi
destination_abs=$(CDPATH= cd "$destination" && pwd -P)
if [ ! -x "$destination/devdash" ] || [ "$("$destination/devdash" version)" != 'devdash v0.1.2' ]; then
	fail "$name" "$output" "local installer did not install the source build"
fi
assert_contains "$name" "$output" "Devdash installed to $destination_abs/devdash"
assert_contains "$name" "$output" "export PATH=\"$destination_abs:\$PATH\""
assert_contains "$name" "$go_log" '|build -o bin/devdash ./cmd/devdash'
pass "$name"

name=install-default-home
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
home=$case_root/home
mkdir -p "$home"
setup_install_fixture "$case_root"
if ! (unset DEVDASH_INSTALL_DIR; HOME=$home FAKE_GO_LOG=$go_log PATH=$fake_bin:$original_path \
	/bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1); then
	fail "$name" "$output" "local installer returned nonzero"
fi
if [ "$("$home/.local/bin/devdash" version)" != 'devdash v0.1.2' ]; then
	fail "$name" "$output" "default HOME destination was not used"
fi
pass "$name"

name=install-empty-destination
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
mkdir -p "$case_root"
setup_install_fixture "$case_root"
if DEVDASH_INSTALL_DIR= FAKE_GO_LOG=$go_log PATH=$fake_bin:$original_path \
	/bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1; then
	fail "$name" "$output" "local installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: install directory is empty'
if [ -e "$go_log" ]; then
	fail "$name" "$output" "build ran before destination validation"
fi
pass "$name"

name=install-missing-home
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
mkdir -p "$case_root"
setup_install_fixture "$case_root"
if (unset HOME DEVDASH_INSTALL_DIR; FAKE_GO_LOG=$go_log PATH=$fake_bin:$original_path \
	/bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1); then
	fail "$name" "$output" "local installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: HOME is not set; set DEVDASH_INSTALL_DIR explicitly'
if [ -e "$go_log" ]; then
	fail "$name" "$output" "build ran before HOME validation"
fi
pass "$name"

name=install-build-failure-preserves-existing
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
destination=$case_root/bin
mkdir -p "$destination"
printf original >"$destination/devdash"
setup_install_fixture "$case_root"
if DEVDASH_INSTALL_DIR=$destination FAKE_GO_LOG=$go_log FAKE_GO_MODE=fail \
	PATH=$fake_bin:$original_path /bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1; then
	fail "$name" "$output" "local installer unexpectedly succeeded"
fi
assert_unchanged "$name" "$output" "$destination/devdash"
pass "$name"

name=install-check-failure-preserves-existing
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
destination=$case_root/bin
mkdir -p "$destination"
printf original >"$destination/devdash"
setup_install_fixture "$case_root"
if DEVDASH_INSTALL_DIR=$destination FAKE_GO_LOG=$go_log FAKE_GO_MODE=bad-help \
	PATH=$fake_bin:$original_path /bin/sh "$install_fixture/scripts/install-local.sh" >"$output" 2>&1; then
	fail "$name" "$output" "local installer unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: locally built devdash failed its executable check'
assert_unchanged "$name" "$output" "$destination/devdash"
pass "$name"

coverage_fixture=$test_root/coverage/repo
mkdir -p "$coverage_fixture/scripts" "$coverage_fixture/test"
cp "$repo_root/install.sh" "$coverage_fixture/install.sh"
cp "$repo_root/scripts/"*.sh "$coverage_fixture/scripts/"
for test_script in install_test.sh tooling_test.sh; do
	cat >"$coverage_fixture/test/$test_script" <<'EOF'
#!/bin/sh
exit 0
EOF
	chmod 0755 "$coverage_fixture/test/$test_script"
done

check_coverage() {
	name=$1
	percentage=$2
	expect_success=$3
	expected_error=$4
	case_root=$test_root/$name
	output=$case_root/output
	go_log=$case_root/go.log
	mkdir -p "$case_root"
	coverage_output="example/file.go:1: function 50.0%
total: (statements) $percentage%"
	if FAKE_GO_LOG=$go_log FAKE_COVERAGE_OUTPUT=$coverage_output PATH=$fake_bin:$original_path \
		/bin/sh "$coverage_fixture/scripts/test.sh" >"$output" 2>&1; then
		result=success
	else
		result=failure
	fi
	if [ "$expect_success" = yes ] && [ "$result" != success ]; then
		fail "$name" "$output" "coverage gate unexpectedly failed"
	fi
	if [ "$expect_success" = no ] && [ "$result" != failure ]; then
		fail "$name" "$output" "coverage gate unexpectedly succeeded"
	fi
	if [ -n "$expected_error" ]; then
		assert_contains "$name" "$output" "$expected_error"
	fi
	assert_contains "$name" "$go_log" 'test ./... -coverprofile='
	pass "$name"
}

check_coverage coverage-above-threshold 80.1 yes ''
check_coverage coverage-equal-threshold 80.0 no 'error: total coverage must be greater than 80.0% (got 80.0%)'

name=coverage-missing-total
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
mkdir -p "$case_root"
if FAKE_GO_LOG=$go_log FAKE_COVERAGE_OUTPUT='example/file.go:1: function 50.0%' \
	PATH=$fake_bin:$original_path /bin/sh "$coverage_fixture/scripts/test.sh" >"$output" 2>&1; then
	fail "$name" "$output" "coverage gate unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: total coverage was not reported'
pass "$name"

setup_release_fixture() {
	release_case_root=$1
	release_fixture=$release_case_root/repo
	mkdir -p "$release_fixture/scripts"
	cp "$repo_root/Makefile" "$release_fixture/Makefile"
	cp "$repo_root/install.sh" "$release_fixture/install.sh"
	cp "$repo_root/scripts/release.sh" "$release_fixture/scripts/release.sh"
	cat >"$release_fixture/scripts/test.sh" <<'EOF'
#!/bin/sh
printf 'called\n' >>"${FAKE_TEST_LOG:-/dev/null}"
exit "${FAKE_TEST_RESULT:-0}"
EOF
	chmod 0755 "$release_fixture/scripts/test.sh"
}

name=release-extra-argument
case_root=$test_root/$name
output=$case_root/output
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if /bin/sh "$release_fixture/scripts/release.sh" unexpected >"$output" 2>&1; then
	fail "$name" "$output" "extra argument unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'usage: scripts/release.sh'
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

name=release-version-prefix-validation
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if FAKE_APP_VERSION=1.2.3 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	PATH=$fake_bin:$original_path /bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "non-v application version unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: application version must start with v (got devdash 1.2.3)'
if [ -e "$test_log" ]; then
	fail "$name" "$output" "test gate ran before version validation"
fi
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

name=release-tag-ref-validation
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if FAKE_APP_VERSION='vbad tag' FAKE_GIT_CHECK_RESULT=1 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	PATH=$fake_bin:$original_path /bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "invalid application tag unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: application version is not a valid tag: vbad tag'
if [ -e "$test_log" ]; then
	fail "$name" "$output" "test gate ran before tag validation"
fi
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

name=release-unsupported-host
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if FAKE_APP_VERSION=v1.2.3 FAKE_UNAME_S=FreeBSD FAKE_UNAME_M=x86_64 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	PATH=$fake_bin:$original_path /bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "unsupported host unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: unsupported release host: FreeBSD/x86_64'
if [ -e "$test_log" ]; then
	fail "$name" "$output" "test gate ran before host validation"
fi
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

name=make-release-propagates-test-failure
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if FAKE_APP_VERSION=v1.2.3 FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	FAKE_TEST_RESULT=1 PATH=$fake_bin:$original_path make -s -C "$release_fixture" release >"$output" 2>&1; then
	fail "$name" "$output" "make release unexpectedly succeeded"
fi
if [ "$(cat "$test_log")" != called ]; then
	fail "$name" "$output" "release did not invoke its test gate"
fi
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

name=release-artifacts
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if ! FAKE_APP_VERSION=v1.2.3 FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	PATH=$fake_bin:$original_path /bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "release script returned nonzero"
fi
dist=$release_fixture/dist
dist_abs=$(CDPATH= cd "$dist" && pwd -P)
assert_contains "$name" "$output" "Release artifacts written to $dist_abs"
expected_assets='devdash_darwin_amd64.tar.gz devdash_darwin_arm64.tar.gz devdash_linux_amd64.tar.gz devdash_linux_arm64.tar.gz checksums.txt'
asset_count=0
for path in "$dist"/*; do
	asset=${path##*/}
	case " $expected_assets " in
		*" $asset "*)
			;;
		*)
			fail "$name" "$output" "unexpected release artifact: $asset"
			;;
	esac
	asset_count=$((asset_count + 1))
done
if [ "$asset_count" -ne 5 ] || [ -e "$dist/marker" ]; then
	fail "$name" "$output" "release output did not replace dist with exactly five assets"
fi
for archive in devdash_darwin_amd64.tar.gz devdash_darwin_arm64.tar.gz devdash_linux_amd64.tar.gz devdash_linux_arm64.tar.gz; do
	if [ "$(tar -tzf "$dist/$archive")" != devdash ]; then
		fail "$name" "$output" "$archive does not contain only root-level devdash"
	fi
	expected_digest=$(sha256_file "$dist/$archive")
	actual_digest=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$dist/checksums.txt")
	if [ "$actual_digest" != "$expected_digest" ]; then
		fail "$name" "$output" "checksum mismatch or missing basename entry for $archive"
	fi
done
if [ "$(awk 'END { print NR }' "$dist/checksums.txt")" -ne 4 ]; then
	fail "$name" "$output" "checksums.txt does not contain exactly four entries"
fi
for target in 'GOOS=darwin|GOARCH=amd64' 'GOOS=darwin|GOARCH=arm64' 'GOOS=linux|GOARCH=amd64' 'GOOS=linux|GOARCH=arm64'; do
	assert_contains "$name" "$go_log" "CGO_ENABLED=0|$target|build -trimpath -ldflags -s -w -o"
done
assert_not_contains "$name" "$go_log" 'internal/app.version='
extract_dir=$case_root/extract
mkdir -p "$extract_dir"
tar -xzf "$dist/devdash_linux_amd64.tar.gz" -C "$extract_dir" devdash
if [ "$("$extract_dir/devdash" version)" != 'devdash v1.2.3' ]; then
	fail "$name" "$output" "release artifact did not report the tracked version"
fi
pass "$name"

name=release-shasum-fallback
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
shasum_log=$case_root/shasum.log
setup_release_fixture "$case_root"
limited_bin=$case_root/bin
mkdir -p "$limited_bin"
for command_name in sh git tar gzip curl awk mktemp chmod cp mv mkdir rm dirname cat; do
	ln -s "$(command -v "$command_name")" "$limited_bin/$command_name"
done
cp "$fake_bin/go" "$limited_bin/go"
cp "$fake_bin/uname" "$limited_bin/uname"
cat >"$limited_bin/shasum" <<'EOF'
#!/bin/sh
printf 'called\n' >>"$SHASUM_LOG"
if [ -n "${REAL_SHA256SUM:-}" ]; then
	if [ "${1:-}" = -a ]; then
		shift 2
	fi
	exec "$REAL_SHA256SUM" "$@"
fi
exec "$REAL_SHASUM" "$@"
EOF
chmod 0755 "$limited_bin/go" "$limited_bin/uname" "$limited_bin/shasum"
if command -v sha256sum >/dev/null 2>&1; then
	real_sha256sum=$(command -v sha256sum)
	real_shasum=
else
	real_sha256sum=
	real_shasum=$(command -v shasum)
fi
if ! FAKE_APP_VERSION=v2.0.0 FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 FAKE_GO_LOG=$go_log FAKE_TEST_LOG=$test_log \
	SHASUM_LOG=$shasum_log REAL_SHA256SUM=$real_sha256sum REAL_SHASUM=$real_shasum \
	PATH=$limited_bin /bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "release failed with shasum fallback"
fi
if [ ! -s "$shasum_log" ]; then
	fail "$name" "$output" "shasum fallback was not used"
fi
pass "$name"

name=release-smoke-failure-preserves-dist
case_root=$test_root/$name
output=$case_root/output
go_log=$case_root/go.log
test_log=$case_root/test.log
setup_release_fixture "$case_root"
mkdir -p "$release_fixture/dist"
printf original >"$release_fixture/dist/marker"
if FAKE_APP_VERSION=v1.2.3 FAKE_UNAME_S=Linux FAKE_UNAME_M=x86_64 FAKE_GO_LOG=$go_log FAKE_GO_MODE=bad-help \
	FAKE_TEST_LOG=$test_log PATH=$fake_bin:$original_path \
	/bin/sh "$release_fixture/scripts/release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "release unexpectedly survived smoke failure"
fi
assert_unchanged "$name" "$output" "$release_fixture/dist/marker"
pass "$name"

setup_publish_fixture() {
	publish_case_root=$1
	publish_fixture=$publish_case_root/repo
	mkdir -p "$publish_fixture/scripts"
	cp "$repo_root/scripts/publish-release.sh" "$publish_fixture/scripts/publish-release.sh"
	cat >"$publish_fixture/scripts/release.sh" <<'EOF'
#!/bin/sh
printf 'release.sh:%s\n' "$*" >>"$PUBLISH_CALL_LOG"
exit "${FAKE_RELEASE_RESULT:-0}"
EOF
	chmod 0755 "$publish_fixture/scripts/release.sh"
}

assert_calls() {
	name=$1
	output_file=$2
	calls_file=$3
	expected=$4
	if [ "$(cat "$calls_file")" != "$expected" ]; then
		fail "$name" "$output_file" "publication calls were not ordered as expected"
	fi
}

publish_assets='dist/devdash_darwin_amd64.tar.gz dist/devdash_darwin_arm64.tar.gz dist/devdash_linux_amd64.tar.gz dist/devdash_linux_arm64.tar.gz dist/checksums.txt'

name=publish-argument-validation
case_root=$test_root/$name
output=$case_root/output
setup_publish_fixture "$case_root"
if /bin/sh "$publish_fixture/scripts/publish-release.sh" >"$output" 2>&1; then
	fail "$name" "$output" "missing commit unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'usage: scripts/publish-release.sh <commit>'
if /bin/sh "$publish_fixture/scripts/publish-release.sh" one two >"$output" 2>&1; then
	fail "$name" "$output" "extra commit unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'usage: scripts/publish-release.sh <commit>'
pass "$name"

name=publish-version-prefix-validation
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if FAKE_APP_VERSION=1.2.3 PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "non-v application version unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: application version must start with v (got devdash 1.2.3)'
assert_calls "$name" "$output" "$calls" 'go:run ./cmd/devdash version'
pass "$name"

name=publish-tag-ref-validation
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if FAKE_APP_VERSION='vbad tag' FAKE_GIT_CHECK_RESULT=1 PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "invalid application tag unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: application version is not a valid tag: vbad tag'
expected_calls='go:run ./cmd/devdash version
git:check-ref-format refs/tags/vbad tag'
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-new-tag
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if ! FAKE_APP_VERSION=v1.2.3 FAKE_RELEASE_EXISTS=no PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "new release publication failed"
fi
expected_calls="go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3
release.sh:
git:tag v1.2.3 commit-1
git:push origin refs/tags/v1.2.3
gh:release create v1.2.3 $publish_assets --verify-tag --generate-notes"
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-same-commit-missing-release
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if ! FAKE_APP_VERSION=v1.2.3 FAKE_TAG_COMMIT=commit-1 FAKE_RELEASE_EXISTS=no \
	PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "same-commit release creation failed"
fi
expected_calls="go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3
release.sh:
gh:release create v1.2.3 $publish_assets --verify-tag --generate-notes"
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-same-commit-existing-release
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if ! FAKE_APP_VERSION=v1.2.3 FAKE_TAG_COMMIT=commit-1 FAKE_RELEASE_EXISTS=yes \
	PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "same-commit release upload failed"
fi
expected_calls="go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3
release.sh:
gh:release upload v1.2.3 $publish_assets --clobber"
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-existing-release-from-other-commit
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if ! FAKE_APP_VERSION=v1.2.3 FAKE_TAG_COMMIT=old-commit FAKE_RELEASE_EXISTS=yes \
	PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "existing release skip failed"
fi
assert_contains "$name" "$output" 'Release v1.2.3 already exists from old-commit; nothing to publish.'
expected_calls='go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3'
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-tag-collision-without-release
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if FAKE_APP_VERSION=v1.2.3 FAKE_TAG_COMMIT=old-commit FAKE_RELEASE_EXISTS=no \
	PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "tag collision unexpectedly succeeded"
fi
assert_contains "$name" "$output" 'error: tag v1.2.3 points to old-commit, not commit-1, and has no release'
expected_calls='go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3'
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

name=publish-build-failure-prevents-tag
case_root=$test_root/$name
output=$case_root/output
calls=$case_root/calls
setup_publish_fixture "$case_root"
if FAKE_APP_VERSION=v1.2.3 FAKE_RELEASE_EXISTS=no FAKE_RELEASE_RESULT=1 \
	PUBLISH_CALL_LOG=$calls PATH=$fake_bin:$original_path \
	/bin/sh "$publish_fixture/scripts/publish-release.sh" commit-1 >"$output" 2>&1; then
	fail "$name" "$output" "failed artifact build unexpectedly published"
fi
expected_calls='go:run ./cmd/devdash version
git:check-ref-format refs/tags/v1.2.3
git:rev-list -n 1 refs/tags/v1.2.3
gh:release view v1.2.3
release.sh:'
assert_calls "$name" "$output" "$calls" "$expected_calls"
pass "$name"

printf 'tooling tests: %d passed\n' "$passed"
