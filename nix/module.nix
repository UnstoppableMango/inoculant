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

  image = "inoculant:${version}";
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

    kubeconfig = lib.mkOption {
      type = lib.types.externalPath;
      default = "/etc/${config.services.kubernetes.pki.etcClusterAdminKubeconfig}";
    };
  };

  config = lib.mkIf cfg.enable {
    services.kubernetes.kubelet.seedDockerImages = [ cfg.imageArchive ];

    systemd.tmpfiles.rules = [
      "d ${cfg.manifestsDirectory} 0755 root root -"
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
            name = "manifests";
            hostPath.path = cfg.manifestsDirectory;
          }
        ];
      };
    };
  };
}
