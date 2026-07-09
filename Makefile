GO        ?= go
GOMOD2NIX ?= gomod2nix
GINKGO    ?= ginkgo

GO_SRC ?= $(shell find . -name '*.go')
NIX_SRC ?= $(shell find . -name '*.nix')

build:
	nix build .#

container: bin/inoculant.tar

test:
	$(GINKGO) run -r

update:
	nix flake update

check lint:
	nix flake check

format fmt:
	nix fmt

tidy: go.sum nix/gomod2nix.toml

bin:
	@mkdir -p $@

bin/inoculant.tar: ${GO_SRC} ${NIX_SRC} | bin
	nix run .#container.copyTo -- "oci-archive:${CURDIR}/$@:latest"

go.sum: go.mod ${GO_SRC}
	$(GO) mod tidy

nix/gomod2nix.toml: go.sum
	$(GOMOD2NIX) generate --dir ${CURDIR} --outdir ${@D}
