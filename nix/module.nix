{
  pkgs,
  lib,
  config,
}:
let
  cfg = config.services.kubernetes.inoculant;

  tarball = pkgs.runCommand "inoculant.tar" { } ''
    ${cfg.skopeo}/bin/skopeo copy \
      nix:${cfg.pkg} \
      oci-archive:$out
  '';
in
{
  options.services.kubernetes.inoculant = {
    enable = lib.mkEnableOption "A kubernetes bootstrapper";
    pkg = lib.mkPackageOption pkgs "inoculant" { };
    skopeo = lib.mkPackageOption pkgs "skopeo-nix2container" { };
  };

  config = lib.mkIf cfg.enable {
    services.kubernetes.kubelet.seedDockerImages = [
      tarball
    ];
  };
}
