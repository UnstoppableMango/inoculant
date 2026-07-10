{ containerFor, skopeoFor }:
{
  pkgs,
  lib,
  config,
}:
let
  cfg = config.services.kubernetes.inoculant;
in
{
  options.services.kubernetes.inoculant = {
    enable = lib.mkEnableOption "A kubernetes bootstrapper";
    pkg = lib.mkPackageOption pkgs "inoculant" { };

    container = lib.mkOption {
      type = lib.types.package;
      default = containerFor pkgs.system;
    };

    skopeo = lib.mkOption {
      type = lib.types.package;
      default = skopeoFor pkgs.system;
    };
  };

  config = lib.mkIf cfg.enable {
    services.kubernetes.kubelet.seedDockerImages = [
      (pkgs.callPackage ./tarball.nix {
        inherit (cfg) container skopeo;
      })
    ];
  };
}
