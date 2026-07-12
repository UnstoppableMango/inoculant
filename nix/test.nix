{ module, testers }:

testers.nixosTest {
  name = "nixos-integration";
  nodes.machine =
    { pkgs, ... }:
    {
      imports = [ module ];

      services.kubernetes = {
        inoculant.enable = true;
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
  '';
}
