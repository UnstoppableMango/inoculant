{
  inputs,
  version,
}:
{
  pkgs,
  lib,
  config,
  ...
}:
let
  inherit (inputs) globset;
  inherit (pkgs.stdenv.hostPlatform) system;
  inherit (inputs.nix2container.packages.${system})
    nix2container
    skopeo-nix2container
    ;

  top = config.services.kubernetes;
  cfg = top.inoculant;

  # A real directory of copies, not symlinks: the pod bind-mounts this
  # directory alone (not the wider Nix store), so entries must not point
  # outside it.
  manifestsDrv = pkgs.runCommand "inoculant-manifests" { } (
    ''
      mkdir -p "$out"
    ''
    + lib.concatStrings (
      lib.mapAttrsToList (name: manifest: ''
        install -Dm444 ${pkgs.writeText "${name}.json" (builtins.toJSON manifest)} "$out/"${lib.escapeShellArg "${name}.json"}
      '') cfg.manifests
    )
    + lib.concatMapStrings (src: ''
      cp -r --no-preserve=mode,ownership ${src} "$out/$(basename ${src})"
      chmod -R a+rX "$out/$(basename ${src})"
    '') cfg.manifestFiles
  );
in
{
  options.services.kubernetes.inoculant = {
    enable = lib.mkEnableOption "A kubernetes bootstrapper";

    # Built from this module's own `pkgs` arg, not the flake's already-built
    # `inoculant` package: this module is exported as flake.nixosModules.default
    # and must stay import-able by any NixOS config on any nixpkgs pin, so it
    # can't reach into this flake's perSystem outputs.
    pkg = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./inoculant.nix {
        inherit globset version;
      };
    };

    # Same self-containment reasoning as `pkg` above: rebuilt from `cfg.pkg`
    # rather than reusing the flake's `container`/tarball packages.
    imageArchive = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./tarball.nix {
        inherit (cfg) skopeo;
        inherit version;

        container = pkgs.callPackage ./container.nix {
          inoculant = cfg.pkg;
          inherit nix2container version;
        };
      };
    };

    skopeo = lib.mkOption {
      type = lib.types.package;
      default = skopeo-nix2container;
    };

    manifestsDirectory = lib.mkOption {
      type = lib.types.externalPath;
      default = "/etc/inoculant/manifests";
      description = "Host directory containing static manifests for inoculant to apply.";
    };

    # Same shape as services.kubernetes.kubelet.manifests: attrset of nix
    # attrs, rendered to "<name>.json" via builtins.toJSON. Keys become
    # filenames (via manifestsDrv below), so they must be plain names (no
    # "/" or other path-breaking characters).
    manifests = lib.mkOption {
      type = lib.types.attrsOf lib.types.attrs;
      default = { };
      description = "Static manifests seeded into manifestsDirectory for inoculant to apply.";
    };

    manifestFiles = lib.mkOption {
      type = lib.types.listOf lib.types.path;
      default = [ ];
      description = "Extra manifest files or directories copied into manifestsDirectory verbatim, alongside `manifests`.";
    };

    allowedGVKs = lib.mkOption {
      type = lib.types.listOf (lib.types.submodule {
        options = {
          group = lib.mkOption {
            type = lib.types.str;
            default = "";
            description = "API group. Empty string for core resources (v1).";
          };
          version = lib.mkOption {
            type = lib.types.str;
            description = "API version (e.g. v1, v1beta1).";
          };
          kind = lib.mkOption {
            type = lib.types.str;
            description = "Resource kind (e.g. ConfigMap, Deployment).";
          };
        };
      });
      default = [ ];
      description = ''
        Additional GVKs inoculant is permitted to apply, beyond those
        auto-derived from `manifests`. Required when using `manifestFiles`.
        Empty combined list (no manifests, no explicit entries) = all GVKs permitted.
      '';
    };
  };

  config = lib.mkIf cfg.enable (
    let
      image = "docker.io/library/inoculant:${version}";

      # Use the cluster-admin kubeconfig that NixOS PKI already provisions.
      # inoculant fills the same role as addonManager's bootstrapAddons phase —
      # first-boot cluster init — so cluster-admin is the appropriate credential.
      kubeconfigFile = top.pki.clusterAdminKubeconfig;

      # Derive permitted GVKs from cfg.manifests (known at eval time) plus any
      # explicit cfg.allowedGVKs entries needed for manifestFiles. Empty list
      # means no --allowed-gvk flags are passed and inoculant permits all GVKs.
      effectiveAllowedGVKs =
        let
          extractGVK = manifest:
            let
              apiVersion = manifest.apiVersion or "";
              kind = manifest.kind or "";
              parts = lib.splitString "/" apiVersion;
              group = if builtins.length parts == 2 then builtins.elemAt parts 0 else "";
              version = if builtins.length parts == 2 then builtins.elemAt parts 1 else apiVersion;
            in
            { inherit group version kind; };
          fromManifests = lib.unique (map extractGVK (lib.attrValues cfg.manifests));
        in
        lib.unique (fromManifests ++ cfg.allowedGVKs);

      allowedGVKArgs = lib.concatMap (gvk:
        let
          gvkStr = if gvk.group != ""
                   then "${gvk.group}/${gvk.version}/${gvk.kind}"
                   else "${gvk.version}/${gvk.kind}";
        in
        [ "--allowed-gvk" gvkStr ]
      ) effectiveAllowedGVKs;
    in
    {
      # TODO: this reimports the archive on every kubelet restart (e.g. cert
      # rotation), not just the first boot. Guard with an existence check.
      # (nixpkgs' own seedDockerImages preStart has the same flaw.)
      systemd.services.kubelet.preStart = lib.mkAfter ''
        ${pkgs.containerd}/bin/ctr -n k8s.io images import --index-name ${image} ${cfg.imageArchive}
      '';

      systemd.tmpfiles.rules = [
        "L+ ${cfg.manifestsDirectory} - - - - ${manifestsDrv}"
      ];

      services.kubernetes.kubelet.manifests.inoculant = {
        apiVersion = "v1";
        kind = "Pod";
        metadata = {
          name = "inoculant";
          namespace = "kube-system";
        };
        spec = {
          restartPolicy = "OnFailure";
          hostNetwork = true;
          dnsPolicy = "Default";
          containers = [
            {
              name = "inoculant";
              image = image;
              args = [
                "--kubeconfig"
                "/etc/inoculant/kubeconfig"
                "apply"
                "/etc/inoculant/manifests"
              ] ++ allowedGVKArgs;
              volumeMounts = [
                {
                  name = "kubeconfig";
                  mountPath = "/etc/inoculant/kubeconfig";
                  readOnly = true;
                }
                {
                  name = "ca-cert";
                  mountPath = top.caFile;
                  readOnly = true;
                }
                {
                  name = "client-cert";
                  mountPath = top.pki.certs.clusterAdmin.cert;
                  readOnly = true;
                }
                {
                  name = "client-key";
                  mountPath = top.pki.certs.clusterAdmin.key;
                  readOnly = true;
                }
                {
                  name = "manifests";
                  mountPath = "/etc/inoculant/manifests";
                  readOnly = true;
                }
              ];
            }
          ];
          volumes = [
            {
              name = "kubeconfig";
              hostPath.path = kubeconfigFile;
            }
            {
              name = "ca-cert";
              hostPath.path = top.caFile;
            }
            {
              name = "client-cert";
              hostPath.path = top.pki.certs.clusterAdmin.cert;
            }
            {
              name = "client-key";
              hostPath.path = top.pki.certs.clusterAdmin.key;
            }
            {
              name = "manifests";
              hostPath.path = cfg.manifestsDirectory;
            }
          ];
        };
      };
    }
  );
}
