# Use case: ephemeral node with persistent identity

Exploratory.
This is a sketch of a deployment shape inoculant does not support, not a committed goal.

A personal desktop contributes its idle capacity to the home cluster.
When the machine is idle it joins and accepts batch work; when the person sits down it drains and stops accepting work.
The machine keeps one identity across every session, so the cluster treats each join as the same node returning rather than a new node appearing.

Inoculant owns the cluster-side half of that transition: the Node object's schedulability, its labels and taints, its scoped credentials, and the node-local manifests that travel with it.
Systemd owns the host-side half: idle detection, kubelet, and ordering.

## Leaving is best-effort

The machine can be powered off, suspended, or unplugged at any moment, and then nothing inoculant would have run on the way out runs at all.
So the design cannot put anything load-bearing in the leave path.

Two consequences shape everything below:

- **The Node object persists by default.** The cluster's handling of a departed node has to be the same whether leave ran or not, and the case that cannot be improved is the one where it did not. A node that goes `NotReady` on its own is therefore the baseline, and a graceful leave is an optimization on top of it: pods move on inoculant's schedule instead of waiting out eviction timers.
- **Join reconciles, leave does not clean up.** Adopting an existing Node object and reconciling it to the declared state is the only path, not a recovery path. Deleting owned objects, resetting labels, and pruning all belong to join, which is guaranteed to run before the node is usable.

`inoculant leave --delete-node` exists for decommissioning a machine on purpose.
It is not what the idle trigger calls.

## What persists and what does not

Persistent, declared in Nix on the desktop:

- Node name (`desk-idle-01`), so scheduling constraints, local PV node affinity, and dashboards resolve to one entity.
- The Node API object itself, which outlives any single session.
- Node labels and taints, including the class label workloads select on and the taint that keeps everything else off.
- The machine's client credentials and CA trust, kept on disk outside the Nix store.
- The node's address on the cluster overlay (WireGuard or Tailscale), so CNI routing is stable across sessions.
- The set of node-local manifests inoculant applies on behalf of this node.

Ephemeral, recreated each session:

- Pods scheduled onto the node.
- The scoped service account token inoculant mints for itself.
- Node status, leases, and conditions.

Identity lives on the desktop and in Nix.
The Node object is a cache of it, and a session is the window where that cache is `Ready`.

## Join sequence

Triggered when the idle watcher decides the machine is free.

1. `wg-quick` (or `tailscaled`) brings up the cluster overlay address.
2. `kubelet.service` starts and re-registers the existing Node object under the declared name, unschedulable and tainted from the start (`--register-with-taints`), so nothing lands before inoculant has finished.
3. `inoculant-bootstrap.service` mints a scoped token kubeconfig for this node using the machine's own credentials.
4. `inoculant-join.service` runs `inoculant join`:
   - reconciles labels and taints to the declared set, removing ones it previously set that are no longer declared;
   - applies the node-scoped manifest set, labeled as owned by this node, pruning objects the set no longer contains;
   - removes the join taint and clears `unschedulable` as its final step, which is the point the node becomes usable.

The node is schedulable only after step 4 completes, so a failed join leaves a cordoned node rather than a half-configured one that accepts work.
Step 4 makes no assumption about how the previous session ended.

## Leave sequence

Triggered when the idle watcher sees activity, or by a shutdown inhibitor that gets far enough to run it.

1. `inoculant leave` cordons the node, which is the one step that matters and the cheapest to complete.
2. It evicts pods through the Eviction API so PodDisruptionBudgets are respected.
3. Eviction runs under a deadline (`--drain-timeout`, default 60s). Pods still present when it expires are deleted with a short grace period, because the person waiting to use their desktop outranks a batch job's cleanup.
4. Systemd stops kubelet, then tears down the overlay address.

The Node object stays, cordoned, and goes `NotReady` once its lease expires.
Nothing is deleted and nothing on the cluster side depends on steps 2 or 3 having run.

Ordering is expressed with a single target, so one `systemctl stop` unwinds the stack in reverse:

```
cluster-member.target
  wants: wg-quick@cluster, kubelet, inoculant-bootstrap, inoculant-join
inoculant-leave.service
  Before=kubelet.service (on stop), ExecStart=inoculant leave
```

Kubelet's graceful node shutdown covers an ordinary `poweroff` via a systemd inhibitor lock, and covers nothing on power loss.
Treat it as the same best-effort tier as `inoculant leave`.

## The cluster has to tolerate a node that vanishes

This is a prerequisite rather than a nicety, because the vanishing case is the common one.

- Workloads targeting the node tolerate `node.kubernetes.io/unreachable` and `not-ready` with a short `tolerationSeconds`, so a disappeared node sheds its pods quickly instead of after the five-minute default.
- Alerting excludes nodes carrying the ephemeral class label from `NodeNotReady` and node-down rules, or the desktop pages someone every evening.
- Anything scheduled here is interruptible, which makes an abrupt loss equivalent to a drain that finished instantly.
- Capacity planning treats the node as absent. It is bonus capacity, never a dependency.

## Surface this adds

CLI:

```
inoculant join  --node-name <name> --label k=v --taint k=v:Effect --manifests <dir>
inoculant leave --node-name <name> --drain-timeout 60s [--delete-node]
```

`join` is a superset of the current `label-node` plus `apply`, with cordon and taint handling around it.
`leave` is new, and outside `--delete-node` it only ever cordons and drains.

NixOS module, under `services.kubernetes.inoculant.ephemeral`:

- `enable`
- `nodeName`
- `taints` (default `unmango.dev/ephemeral=true:NoSchedule`)
- `drainTimeout`
- `idleTrigger` (`logind`, `swayidle`, or `none` for a manually driven node)

The existing `nodeLabels` option carries over unchanged.

## Cluster-side prerequisites

The desktop cannot hold cluster-admin, which is what the control-plane static pod path assumes today.
This use case needs a second credential path:

- A durable client certificate or bootstrap token for kubelet, provisioned once for the node name and kept on the machine.
- A pre-provisioned service account on the control plane, bound to a role that permits exactly what join and leave need: get/patch on the node, create on eviction subresources, and apply on the GVKs in the node-scoped manifest set. Delete on the node is only needed if `--delete-node` is used.
- Inoculant's bootstrap step on this node exchanges the machine credential for that service account's token, rather than minting RBAC from cluster-admin.

Without this, every idle desktop is a cluster-admin credential sitting on a workstation.

## Failure modes worth designing for

- **No leave at all.** Power loss, a hard reset, or a suspend that outruns the inhibitor. Handled by the node persisting and by the tolerations above, not by inoculant.
- **Eviction that cannot finish.** A pod with a PDB that forbids disruption blocks graceful drain forever. The deadline plus forced deletion makes leave bounded, and the forced deletions are reported.
- **Prune collision between nodes.** Today every applied object carries one global `inoculant.unmango.dev/managed-by: inoculant` label, and apply prunes anything with that label outside the current desired set. Two nodes running inoculant with different manifest sets would delete each other's objects. Per-node ownership (`inoculant.unmango.dev/owner: <node-name>`) scoped into the apply-set query is a prerequisite, not an optimization.
- **Overlapping join and leave.** Idle state can flap. Both subcommands take a lease or lock keyed by node name so the second invocation waits or fails fast rather than interleaving a drain with an uncordon.
- **Stale token on rejoin.** The scoped token is short-lived and reminted per session, so a long suspend does not resume with an expired credential.
- **Cordoned node left behind.** A join that fails after kubelet has registered leaves the node `Ready` and unschedulable. That is the intended end state for a failed join, and it means "cordoned" cannot be read as "someone drained this".

## Changes required in inoculant

1. Per-node ownership label and applyset scoping in `internal/apply`.
2. New `internal/node` package for cordon, taint reconciliation, and optional Node deletion.
3. `join` and `leave` subcommands in `cmd/inoculant`.
4. Eviction-based drain with a deadline and a forced-deletion fallback.
5. A credential path that does not require cluster-admin on the joining machine.
6. NixOS module: systemd units and the target above, as an alternative to the static pod deployment mode.

Items 1 and 5 are the load-bearing ones; the rest is mechanical once they exist.

## Non-goals

- Deciding when the machine is idle. That is logind, swayidle, or a cron window, and it lives outside inoculant.
- Guaranteeing a graceful departure. Leave improves the common case and the design stays correct without it.
- Keeping workloads alive across a leave. Anything scheduled here is interruptible by definition.
- Managing kubelet's own configuration or certificates beyond consuming what is already provisioned.
- Continuous reconciliation while the node is joined. Join and leave are events, and inoculant stays one-shot at each of them.
