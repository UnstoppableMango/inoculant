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
  };

  config = lib.mkIf cfg.enable (
    let
      image = "docker.io/library/inoculant:${version}";

      # A Nix store path, not an /etc file. Cert rotation doesn't
      # trigger a new kubeconfig file. mirrors how nixpkgs' own
      # kubelet/kube-proxy/scheduler/controller-manager consume their
      # kubeconfigs,
      kubeconfigFile = top.lib.mkKubeConfig "inoculant" {
        server = top.apiserverAddress;
        certFile = top.pki.certs.inoculant.cert;
        keyFile = top.pki.certs.inoculant.key;
      };
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

      # Own x509 identity instead of reusing the shared cluster-admin
      # kubeconfig, similar to how addonManager is implemented
      services.kubernetes.pki.certs.inoculant = top.lib.mkCert {
        name = "inoculant";
        CN = "inoculant";
        fields.O = "system:masters";
      };

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
              ];
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
                  mountPath = top.pki.certs.inoculant.cert;
                  readOnly = true;
                }
                {
                  name = "client-key";
                  mountPath = top.pki.certs.inoculant.key;
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
              hostPath.path = top.pki.certs.inoculant.cert;
            }
            {
              name = "client-key";
              hostPath.path = top.pki.certs.inoculant.key;
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
