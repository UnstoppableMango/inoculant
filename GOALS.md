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

### 8. Re-apply on change

Inoculant is one-shot, but changes to managed inputs must still reach the
cluster.
Inoculant runs as a kubelet static pod, and kubelet derives a static pod's
identity from a hash of its manifest file: any change to that manifest
recreates the pod, which re-runs bootstrap + apply.
The NixOS module exploits this so that a `nixos-rebuild switch` propagates
changes without any long-running watcher or reconciler.

- **Manifest changes.** The module hashes the seeded manifest set and embeds the
  hash as the `inoculant.unmango.dev/manifests-hash` pod annotation. Editing any
  manifest changes the hash, changes the static pod manifest, and kubelet
  recreates the pod.
- **Inoculant changes.** The module content-addresses the image reference
  (`inoculant:<version>-<imageHash>`). A rebuilt binary yields a new reference,
  which both changes the static pod manifest (triggering recreation) and ensures
  the freshly built image is the one imported into containerd and served, rather
  than a stale same-tag image.

Re-apply is safe because it is idempotent: the main container uses server-side
apply (field manager `inoculant`), so re-applying unchanged manifests is a
no-op and changed fields are updated in place. The bootstrap init container
likewise re-applies idempotent RBAC and mints a fresh short-lived token each
run.

Workflow: edit manifests (or rebuild inoculant) → `nixos-rebuild switch` →
kubelet recreates the inoculant static pod → manifests are re-applied.

> This ordering guarantee holds only while `manifestsDirectory` stays under
> `/etc` (the default). The module writes that path via `environment.etc`, the
> same activation phase that writes the static pod manifest, so both land
> together. A `manifestsDirectory` outside `/etc` falls back to
> `systemd.tmpfiles.rules`, which is applied in a later activation phase, after
> kubelet can already see the new pod manifest, so a recreated pod could
> briefly re-apply stale content.

> Pruning is implemented. Every object inoculant applies is labeled
> `inoculant.unmango.dev/managed-by: inoculant`. After applying the current
> manifest set, `Apply` discovers all API resource types the cluster
> exposes, lists each by that label, and deletes anything not in the
> current desired set. This stays within one-shot semantics (prune-on-apply,
> not continuous reconciliation) and so does not conflict with the
> drift-detection non-goal below.
>
> Limitation: the bootstrap init container mints a token scoped only to the
> GVKs derived from the *current* manifest set (see "Re-apply on change"
> below). If every manifest of a given Kind is removed at once, the token
> has no RBAC left to discover or delete the orphaned objects of that Kind,
> and pruning silently skips them (logged, not fatal). Objects of a Kind
> still used elsewhere in the manifest set are pruned normally. Closing this
> gap means granting broader/previous-generation RBAC (e.g. via
> `additionalAllowedGVKs`) and is left as a follow-up.

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
