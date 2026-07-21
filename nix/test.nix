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
  '';
}
