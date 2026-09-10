{ module, testers }:

testers.nixosTest {
  name = "nixos-integration";
  nodes.machine =
    { pkgs, ... }:
    {
      imports = [ module ];

      services.kubernetes = {
        inoculant.enable = true;
        inoculant.manifests = {
          marker = {
            apiVersion = "v1";
            kind = "ConfigMap";
            metadata.name = "inoculant-marker";
            data = { };
          };
          # Exercises the either-attrs-or-list form: multiple manifests in one "pair.json" file.
          pair = [
            {
              apiVersion = "v1";
              kind = "ConfigMap";
              metadata.name = "inoculant-pair-a";
              data = { };
            }
            {
              apiVersion = "v1";
              kind = "ConfigMap";
              metadata.name = "inoculant-pair-b";
              data = { };
            }
          ];
        };
        # Exercises manifestFiles' raw multi-document YAML support.
        inoculant.manifestFiles = [
          (pkgs.writeText "extra-markers.yaml" ''
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: inoculant-extra-marker-a
            ---
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: inoculant-extra-marker-b
          '')
        ];

        roles = [
          "master"
          "node"
        ];
        masterAddress = "machine";
        easyCerts = true;
      };

      # Exercises re-apply on a manifest change: a second configuration, built
      # as part of the same system closure, that adds one manifest to the seeded
      # set. testScript switches to this mid-test to verify kubelet actually
      # recreates the inoculant pod and re-applies with the new content, rather
      # than just checking the static properties of the first-boot pod.
      specialisation.manifestsChanged.configuration = {
        services.kubernetes.inoculant.manifests.reapplyProbe = {
          apiVersion = "v1";
          kind = "ConfigMap";
          metadata.name = "inoculant-reapply-probe";
          data = { };
        };
      };

      environment.systemPackages = [
        pkgs.kubectl
        pkgs.containerd
      ];

      networking.firewall.enable = false;

      virtualisation = {
        memorySize = 4096;
        diskSize = 4096;
        cores = 2;
      };
    };

  testScript = ''
    machine.start()
    machine.wait_for_unit("kubernetes.target")
    machine.wait_until_succeeds(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get nodes | grep -w Ready"
    )
    machine.succeed("ctr --namespace k8s.io images list | grep inoculant")
    machine.wait_until_succeeds(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-marker",
        timeout=60,
    )
    machine.wait_until_succeeds(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-pair-a",
        timeout=60,
    )
    machine.wait_until_succeeds(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-pair-b",
        timeout=60,
    )
    machine.succeed(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-extra-marker-a"
    )
    machine.succeed(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-extra-marker-b"
    )

    # Shell snippet resolving the inoculant static pod's mirror pod, named
    # inoculant-<node>; excludes the unrelated inoculant-seed-test workload pod.
    # Reused below both for the static-property checks and the recreation check.
    find_pod = (
        "pod=$(kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig -n kube-system "
        "get pods -o name | grep -E '^pod/inoculant-' | grep -v inoculant-seed-test | head -1); "
        "test -n \"$pod\""
    )

    # Re-apply wiring: the static pod must carry the manifests-hash annotation
    # (manifest-change trigger) and a content-addressed image reference
    # (binary-change trigger). A change to either makes kubelet recreate the pod
    # and re-run bootstrap + apply on the next `nixos-rebuild switch`.
    machine.wait_until_succeeds(
        "set -e; "
        + find_pod
        + "; "
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig -n kube-system "
        "get \"$pod\" -o jsonpath=\"{.metadata.annotations['inoculant\\.unmango\\.dev/manifests-hash']}\" | grep -E '^[0-9a-f]{64}$'; "
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig -n kube-system "
        "get \"$pod\" -o jsonpath='{.spec.containers[0].image}' | grep -E 'inoculant:.+-[0-9a-f]{16}$'",
        timeout=90,
    )

    # Re-apply on manifest change: switch to a specialisation that adds one
    # manifest, and confirm kubelet actually recreates the pod (new UID, not
    # just a restarted container) and that the recreated pod re-applies the new
    # manifest content. This is what would catch a regression where the
    # manifestsDirectory update lands in a later activation phase than the
    # static pod manifest write, so a recreated pod mounts stale content.
    baseline_uid = machine.succeed(
        "set -e; "
        + find_pod
        + "; "
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig -n kube-system "
        "get \"$pod\" -o jsonpath='{.metadata.uid}'"
    ).strip()

    machine.succeed(
        "/run/current-system/specialisation/manifestsChanged/bin/switch-to-configuration switch"
    )

    machine.wait_until_succeeds(
        "set -e; "
        + find_pod
        + "; "
        "uid=$(kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig -n kube-system "
        "get \"$pod\" -o jsonpath='{.metadata.uid}'); "
        f"test \"$uid\" != {baseline_uid!r}",
        timeout=90,
    )
    machine.wait_until_succeeds(
        "kubectl --kubeconfig=/etc/kubernetes/cluster-admin.kubeconfig get configmap inoculant-reapply-probe",
        timeout=60,
    )
  '';
}
