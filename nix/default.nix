{
  globset,
  module,
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

  test = pkgs.callPackage ./test.nix {
    inherit module;
  };
in
{
  inherit inoculant container test;
}
