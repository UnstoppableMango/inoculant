{
  globset,
  pkgs,
  nix2container,
  version,
}:
let
  inherit (import ./lib.nix) mkInoculant mkContainer;

  inoculant = mkInoculant {
    inherit pkgs globset version;
    inherit (pkgs) buildGoApplication;
  };

  container = mkContainer {
    inherit
      pkgs
      inoculant
      nix2container
      version
      ;
  };
in
{
  inherit inoculant container;
}
