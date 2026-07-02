{
  description = "A Nix flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    systems.url = "github:nix-systems/default";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };

    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.inputs.systems.follows = "systems";
    };
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import inputs.systems;
      imports = with inputs; [ treefmt-nix.flakeModule ];

      perSystem =
        { pkgs, system, ... }:
        let
          version = "0.0.1";
          kubebuilderAssets = pkgs.runCommand "kubebuilder-assets" { } ''
            mkdir -p $out
            ln -s ${pkgs.etcd}/bin/etcd              $out/etcd
            ln -s ${pkgs.kubernetes}/bin/kube-apiserver $out/kube-apiserver
          '';
        in
        {
          _module.args.pkgs = import inputs.nixpkgs {
            inherit system;
            overlays = with inputs; [ gomod2nix.overlays.default ];
          };

          packages.default = pkgs.callPackage ./nix { inherit version; };

          checks = pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
            nixos-inoculant = pkgs.nixosTest {
              name = "inoculant-nixos-integration";
              nodes.machine =
                { pkgs, ... }:
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
            };
          };

          devShells.default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              direnv
              go
              gomod2nix
              gopls
              ginkgo
              gnumake
              nixfmt
            ];

            GO = "${pkgs.go}/bin/go";
            GOMOD2NIX = "${pkgs.gomod2nix}/bin/gomod2nix";
            GINKGO = "${pkgs.ginkgo}/bin/ginkgo";
            KUBEBUILDER_ASSETS = pkgs.lib.optionalString pkgs.stdenv.isLinux "${kubebuilderAssets}";
          };

          treefmt.programs = {
            actionlint.enable = true;
            nixfmt.enable = true;
            gofmt.enable = true;
          };
        };
    };
}
