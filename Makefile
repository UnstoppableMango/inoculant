GO        ?= go
GOMOD2NIX ?= gomod2nix
GINKGO    ?= ginkgo

GO_SRC ?= $(shell find . -name '*.go')

build:
	nix build .#

container:
	nix build .#container

container-tarball:
	mkdir -p bin
	nix run .#container.copyTo -- "oci-archive:${CURDIR}/bin/inoculant.tar:latest"

test:
	$(GINKGO) run -r

update:
	nix flake update

check lint:
	nix flake check

format fmt:
	nix fmt

tidy: go.sum nix/gomod2nix.toml

go.sum: go.mod ${GO_SRC}
	$(GO) mod tidy

nix/gomod2nix.toml: go.sum
	$(GOMOD2NIX) generate --dir ${CURDIR} --outdir ${@D}
