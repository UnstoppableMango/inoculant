GO_SRC ?= $(shell find . -name '*.go')
NIX_SRC ?= $(shell find . -name '*.nix')

build:
	nix build .#

container: bin/inoculant.tar

test:
	go tool ginkgo run -r

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

bin/manifest.json: ${GO_SRC} ${NIX_SRC} | bin
	nix build .#container --out-link $@

go.sum: go.mod ${GO_SRC}
	go mod tidy

nix/gomod2nix.toml: go.sum
	gomod2nix generate --dir ${CURDIR} --outdir ${@D}
