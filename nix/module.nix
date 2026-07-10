{ skopeo-nix2container }:
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

    skopeo = lib.mkOption {
      type = lib.types.package;
      default = skopeo-nix2container;
    };
  };

  config = lib.mkIf cfg.enable {
    services.kubernetes.kubelet.seedDockerImages = [
      (pkgs.callPackage ./tarball.nix {
        inherit (cfg) skopeo;
        # TODO: container = [...]
      })
    ];
  };
}
