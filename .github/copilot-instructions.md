# Copilot Instructions — inoculant

Go-based, one-shot Kubernetes bootstrapping tool: it connects to a cluster via
kubeconfig, applies static resources, and exits. Reconciliation is delegated to
whatever GitOps tooling it installs. Early development (v0.0.1); `main.go` is
still a stub.

## Commands

The build is Nix-first. Prefer `make` targets (they wrap Nix and the dev-shell tools):

```bash
make build   # nix build .#            (static binary via gomod2nix)
make test    # ginkgo run -r           (all specs)
make lint    # nix flake check         (also runs treefmt: gofmt, nixfmt, actionlint)
make fmt     # nix fmt
make tidy    # go mod tidy
make update  # nix flake update
```

Run a single spec file: `ginkgo run tests/apply_test.go`
Run specs matching a description: `ginkgo run -r --focus "applies a ConfigMap"`

Enter the dev shell first (`nix develop`, or use `direnv`) — it provides Go,
gopls, ginkgo, gomod2nix, and formatters, and sets `KUBEBUILDER_ASSETS` for
envtest on Linux. Running `ginkgo` outside the shell will fail without those.

## Architecture

- `main.go` — entry point, currently empty.
- `tests/suite_test.go` — Ginkgo bootstrap. `BeforeSuite` starts a **real**
  etcd + kube-apiserver via `controller-runtime/envtest` and exposes a shared
  `cfg *rest.Config` and `ctx`. No mocks.
- `tests/apply_test.go` — BDD specs that build a `client-go` clientset from
  `cfg` and act against the live envtest cluster.
- `nix/default.nix` — `buildGoApplication` derivation (static binary).
- `nix/gomod2nix.toml` — Go modules mapped to Nix (schema 3).
- `flake.nix` — dev shell, treefmt config, and a NixOS integration test
  (`checks.nixos-inoculant`) that boots k3s; its assertions are still TODO.

CI (`.github/workflows/ci.yml`): `nix flake check` → `nix build .#`, then a
separate job runs `nix develop -c make test`. Cachix cache: `unstoppablemango`.

## Conventions

- **Regenerate Nix module lock after dependency changes.** Changing
  `go.mod`/`go.sum` requires running `gomod2nix` to refresh
  `nix/gomod2nix.toml`, or `nix build` will drift. `make tidy` handles `go.sum`;
  the Makefile regenerates `nix/gomod2nix.toml` from it.
- **Tests use envtest, not mocks.** New tests belong in `tests/` (package
  `integration_test`) and should reuse the suite-wide `cfg`/`ctx` rather than
  starting their own apiserver.
- **Formatting is enforced via treefmt / `nix flake check`.** Go uses gofmt,
  Nix uses nixfmt, workflows are checked with actionlint. `.editorconfig`: tabs
  for Go, 2-space for `.nix` and YAML.
- **Stay distribution-agnostic.** Connect only through kubeconfig; do not couple
  to a specific distro (k3s/kubeadm/k0s).
- **Non-goals (v1):** ongoing reconciliation/drift detection, secret
  management, multi-cluster, and dependency ordering between resources. See
  `GOALS.md` before adding features in these areas.

## Roadmap (GOALS.md)

v1: apply raw YAML/JSON manifest directories, relying on Kubernetes eventual
consistency (no ordering/retry logic in inoculant). Post-v1: Helm (OCI +
air-gapped charts) and Kustomize as first-class resource types, plus a NixOS
module that generates on-disk config inoculant reads at runtime.
