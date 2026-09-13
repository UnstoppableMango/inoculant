{
  description = "A kubernetes bootstrapper";

  nixConfig = {
    allow-import-from-derivation = false;
    extra-substituters = [
      "https://mangopkgs.cachix.org"
    ];
    extra-trusted-public-keys = [
      "mangopkgs.cachix.org-1:uJ5FgSbOg1uiXLcL0gBh1lO+y3KVuthy6UeOFYR1fLk="
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
            TEST_ASSET_ETCD = "${pkgs.etcd}/bin/etcd";
            TEST_ASSET_KUBECTL = "${pkgs.kubectl}/bin/kubectl";
            TEST_ASSET_KUBE_APISERVER = lib.optionalString pkgs.stdenv.hostPlatform.isLinux "${pkgs.kubernetes}/bin/kube-apiserver";
          };

          treefmt.programs = {
            actionlint.enable = true;
            nixfmt.enable = true;
            gofmt.enable = true;
          };
        };
    };
}
