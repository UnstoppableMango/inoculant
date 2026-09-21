{
  description = "A kubernetes bootstrapper";

  nixConfig = {
    allow-import-from-derivation = false;
    extra-substituters = [
      "https://mangopkgs.cachix.org"
      "https://unstoppablemango.cachix.org"
    ];
    extra-trusted-public-keys = [
      "mangopkgs.cachix.org-1:uJ5FgSbOg1uiXLcL0gBh1lO+y3KVuthy6UeOFYR1fLk="
      "unstoppablemango.cachix.org-1:m7uEI6X1Ov8DyFWJQX4WsRFRWFuzRW5c/Xms8ZaP74U="
    ];
  };

  inputs = {
    # nixpkgs and nix2container follow mangopkgs so that skopeo-nix2container,
    # which nix/module.nix defaults the skopeo option to, resolves to the store
    # path mangopkgs builds and pushes to its cachix cache. Pinning them
    # independently means rebuilding skopeo from source.
    mangopkgs.url = "github:unmango/pkgs";
    nixpkgs.follows = "mangopkgs/nixpkgs";
    systems.url = "github:UnstoppableMango/nix-systems";

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

    nix2container.follows = "mangopkgs/nix2container";

    # No nixpkgs follow: kubepkgs' own pin keeps its derivations identical to
    # what its CI pushes to unstoppablemango.cachix.org, so Kubernetes binaries
    # substitute instead of compiling. x86_64-linux is not pushed yet
    # (unmango/kubepkgs#50) and still compiles.
    kubepkgs = {
      url = "github:unmango/kubepkgs";
      inputs.systems.follows = "systems";
      inputs.flake-parts.follows = "flake-parts";
      inputs.globset.follows = "globset";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };
  };

  outputs =
    inputs@{ flake-parts, ... }:
    let
      version = "0.0.1";
      module = import ./nix/module.nix {
        inherit inputs version;
      };
    in
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import inputs.systems;
      imports = with inputs; [
        systems.flakeModule
        treefmt-nix.flakeModule
      ];

      flake.nixosModules.default = module;

      perSystem =
        {
          inputs',
          pkgs,
          lib,
          system,
          ...
        }:
        let
          inherit (inputs'.nix2container.packages) nix2container;

          # Keep the minor in step with the k8s.io/* client libraries in go.mod.
          k8s = inputs'.kubepkgs.legacyPackages.kubernetes."1.37";

          inherit
            (pkgs.callPackage ./nix {
              inherit (inputs) globset;
              inherit nix2container version;
            })
            inoculant
            container
            ;

          test = pkgs.callPackage ./nix/test.nix {
            inherit module;
            inherit (k8s) kubectl kubernetes;
            etcd = k8s.deps.etcd;
          };
        in
        {
          _module.args.pkgs = import inputs.nixpkgs {
            inherit system;
            overlays = with inputs; [ gomod2nix.overlays.default ];
          };

          packages = {
            inherit container inoculant;
            default = inoculant;
          };

          checks = pkgs.lib.optionalAttrs (system == "x86_64-linux") {
            nixos = test;
          };

          devShells.default = pkgs.mkShellNoCC {
            packages =
              with pkgs;
              [
                direnv
                go
                gomod2nix
                gopls
                ginkgo
                gnumake
                nixfmt
                skopeo
                watchexec
              ]
              ++ lib.optionals pkgs.stdenv.hostPlatform.isLinux [ containerd ];

            # https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest#pkg-constants
            # kubepkgs builds etcd and kube-apiserver for Linux only.
            TEST_ASSET_ETCD = lib.optionalString pkgs.stdenv.hostPlatform.isLinux (lib.getExe k8s.deps.etcd);
            TEST_ASSET_KUBECTL = lib.getExe k8s.kubectl;
            TEST_ASSET_KUBE_APISERVER = lib.optionalString pkgs.stdenv.hostPlatform.isLinux (
              lib.getExe k8s.kube-apiserver
            );
          };

          treefmt.programs = {
            actionlint.enable = true;
            nixfmt.enable = true;
            gofmt.enable = true;
          };
        };
    };
}
