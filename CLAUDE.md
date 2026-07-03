# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go-based Kubernetes bootstrapping tool. One-shot: runs, applies static resources to a cluster, exits. Distribution-agnostic via kubeconfig. Early development (v0.0.1).

## Commands

```bash
make build    # nix build .#
make test     # ginkgo run -r
make lint     # nix flake check
make fmt      # nix fmt
make tidy     # go mod tidy
make update   # nix flake update
```

Run a single test file: `ginkgo run tests/apply_test.go`

Dev shell: `nix develop` (provides Go, gopls, ginkgo, gomod2nix, formatters)

## Architecture

```
main.go           # entry point (minimal)
tests/
  suite_test.go   # envtest bootstrap (controller-runtime etcd + apiserver)
  apply_test.go   # Ginkgo BDD tests against live envtest cluster
nix/
  default.nix     # buildGoApplication (static binary)
  gomod2nix.toml  # go modules → nix (schema 3); regenerate with gomod2nix
flake.nix         # dev shell, treefmt, NixOS integration test (k3s, WIP)
```

**Testing**: Ginkgo v2 + Gomega. Tests spin up real etcd + kube-apiserver via `controller-runtime/envtest` — no mocks. NixOS integration test in `flake.nix` checks: section (currently empty, TODO).

**Build**: Nix-first. Go binary built with `gomod2nix`-generated Nix derivation. After changing `go.mod`/`go.sum`, run `gomod2nix` to regenerate `nix/gomod2nix.toml`.

**CI**: GitHub Actions — `nix flake check` → `nix build .#` → `nix develop -c make test`. Cachix used for build caching (`unstoppablemango` cache).

## Key Dependencies

- `k8s.io/client-go` + `k8s.io/api` v0.36.2 — Kubernetes API client
- `sigs.k8s.io/controller-runtime` v0.24.1 — envtest + operator utilities
- `github.com/onsi/ginkgo/v2` + `gomega` — test framework

## Roadmap (from GOALS.md)

v1: raw manifest directories (YAML/JSON). Post-v1: Helm (OCI), Kustomize. NixOS module planned. Non-goals: multi-cluster, secret management, dependency ordering.
