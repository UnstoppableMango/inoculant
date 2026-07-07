{
  description = "A Nix flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    systems.url = "github:nix-systems/default";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };

    globset = {
      url = "github:pdtpartners/globset";
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
        {
          pkgs,
          lib,
          system,
          ...
        }:
        let
          version = "0.0.1";

          inoculant = pkgs.callPackage ./nix {
            inherit version;
            inherit (inputs) globset;
          };
        in
        {
          _module.args.pkgs = import inputs.nixpkgs {
            inherit system;
            overlays = with inputs; [ gomod2nix.overlays.default ];
          };

          packages = {
            inherit inoculant;
            default = inoculant;
          };

          checks = pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
            nixos = inoculant.test;
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

            # https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest#pkg-constants
            TEST_ASSET_ETCD = "${pkgs.etcd}/bin/etcd";
            TEST_ASSET_KUBECTL = "${pkgs.kubectl}/bin/kubectl";
            TEST_ASSET_KUBE_APISERVER = "${pkgs.kubernetes}/bin/kube-apiserver";
          };

          treefmt.programs = {
            actionlint.enable = true;
            nixfmt.enable = true;
            gofmt.enable = true;
          };
        };
    };
}
