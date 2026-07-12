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

  cfg = config.services.kubernetes.inoculant;

  imageBaseName = "docker.io/library/inoculant";
  image = "${imageBaseName}:${version}";

  # A real directory of copies, not symlinks: the pod bind-mounts this
  # directory alone (not the wider Nix store), so entries must not point
  # outside it.
  manifestsDrv = pkgs.runCommand "inoculant-manifests" { } ''
    mkdir -p "$out"
    ${lib.concatStrings (
      lib.mapAttrsToList (name: text: ''
        cp ${pkgs.writeText name text} "$out/"${lib.escapeShellArg name}
      '') cfg.manifests
    )}
  '';
in
{
  options.services.kubernetes.inoculant = {
    enable = lib.mkEnableOption "A kubernetes bootstrapper";

    pkg = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./inoculant.nix {
        inherit globset version;
      };
    };

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

    manifests = lib.mkOption {
      type = lib.types.attrsOf lib.types.lines;
      default = {
        "marker.yaml" = ''
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: inoculant-marker
          data: {}
        '';
      };
      description = "Static manifests seeded into manifestsDirectory for inoculant to apply.";
    };

    kubeconfig = lib.mkOption {
      type = lib.types.externalPath;
      default = "/etc/${config.services.kubernetes.pki.etcClusterAdminKubeconfig}";
    };
  };

  config = lib.mkIf cfg.enable {
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
              cfg.kubeconfig
              "apply"
              "/manifests"
            ];
            volumeMounts = [
              {
                name = "kubeconfig";
                mountPath = cfg.kubeconfig;
                readOnly = true;
              }
              {
                name = "secrets";
                mountPath = config.services.kubernetes.secretsPath;
                readOnly = true;
              }
              {
                name = "manifests";
                mountPath = "/manifests";
                readOnly = true;
              }
            ];
          }
        ];
        volumes = [
          {
            name = "kubeconfig";
            hostPath.path = cfg.kubeconfig;
          }
          {
            name = "secrets";
            hostPath.path = config.services.kubernetes.secretsPath;
          }
          {
            name = "manifests";
            hostPath.path = cfg.manifestsDirectory;
          }
        ];
      };
    };
  };
}
