.PHONY: build install test release

build:
	sh scripts/build.sh

install:
	sh scripts/install-local.sh

test:
	sh scripts/test.sh

release:
	sh scripts/release.sh "$(VERSION)"
