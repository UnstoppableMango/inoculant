# Inoculant — Project Goals

## Purpose

Inoculant bootstraps Kubernetes clusters by applying static resources at node
initialization time. It fills the gap between a bare kubelet and a running
GitOps operator: install the minimum set of addons (CNI, storage, secrets
manager, GitOps operator) so the cluster can manage itself going forward.

Inoculant is intentionally one-shot. It runs, applies resources, and exits.
Ongoing reconciliation is delegated to whatever GitOps tooling it installs.

## Inspiration

- k3s `/var/lib/rancher/k3s/server/manifests` — static manifest deployment
- Kubernetes addonmanager — apply manifests alongside kubelet
- kubelet static pods — node-local resource lifecycle

## Core Goals

### 1. Distribution-agnostic bootstrap

Connect to any Kubernetes cluster via kubeconfig. No coupling to a specific
distribution (k3s, kubeadm, k0s). Works wherever a valid kubeconfig exists.

### 2. Raw manifest support (v1)

Apply plain YAML/JSON Kubernetes manifests from a configured directory.
Rely on Kubernetes eventual consistency — no explicit ordering or retry logic
in inoculant itself.

### 3. Helm support (post-v1)

- Install and upgrade Helm releases
- Pull charts from OCI registries
- Support air-gapped / local chart bundles (no internet required)

### 4. Kustomize support (post-v1)

Apply kustomize overlays as a first-class resource type alongside manifests
and Helm releases.

### 5. NixOS-native configuration

Expose a NixOS module that integrates with `services.kubernetes` (NixOS
upstream k8s support). The module generates configuration files on disk;
inoculant reads those files at runtime. Users declare everything in Nix —
no separate config format to learn.

### 6. Dual deployment modes

**Systemd service** — inoculant runs as a oneshot systemd unit on the host,
ordered after kubelet is ready.

**Containerized** — inoculant runs as a Docker/Podman container before k8s
is fully up, mounting host paths for kubeconfig and resource directories.

### 7. Static pod compatibility

Operate alongside kubelet's static pod mechanism. Do not conflict with
resources already managed via static pod manifests.

## Non-Goals (v1)

- Ongoing reconciliation / drift detection (use Flux, ArgoCD, etc.)
- Secret management or decryption (use agenix, sops-nix, or external-secrets)
- Multi-cluster management
- Dependency ordering between resources
- Replacing GitOps tooling long-term

## Success Criteria

A NixOS node running inoculant should, after a single boot:

1. Have all declared manifests applied to the cluster
2. Have inoculant exit cleanly with a zero status
3. Leave behind no persistent processes
4. Be reproducible — running again on a fresh node produces identical cluster state
