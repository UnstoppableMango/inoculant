{ testers }:

testers.nixosTest {
  name = "nixos-integration";
  nodes.machine =
    { ... }:
    {
      services.k3s = {
        enable = true;
        role = "server";
        token = "inoculant-test-token";
      };
      virtualisation.memorySize = 2048;
    };

  testScript = ''
    machine.start()
    machine.wait_for_unit("k3s.service", timeout=120)
    # TODO: once NixOS module exists:
    #   machine.succeed("inoculant --kubeconfig /etc/rancher/k3s/k3s.yaml apply /etc/inoculant/manifests")
    #   machine.succeed("kubectl get configmap inoculant-marker")
  '';
}
