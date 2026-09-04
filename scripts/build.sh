#!/bin/sh

set -eu

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
cd "$repo_root"

mkdir -p bin
go build -o bin/devdash ./cmd/devdash
