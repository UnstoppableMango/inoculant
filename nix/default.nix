{
  globset,
  pkgs,
  nix2container,
  version,
}:
let
  inoculant = pkgs.callPackage ./inoculant.nix {
    inherit globset version;
  };

  container = pkgs.callPackage ./container.nix {
    inherit inoculant nix2container version;
  };
in
{
  inherit inoculant container;
}
