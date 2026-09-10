{
  mkInoculant =
    {
      pkgs,
      globset,
      version,
      buildGoApplication,
    }:
    pkgs.callPackage ./inoculant.nix {
      inherit globset version buildGoApplication;
    };

  mkContainer =
    {
      pkgs,
      inoculant,
      nix2container,
      version,
    }:
    pkgs.callPackage ./container.nix {
      inherit inoculant nix2container version;
    };
}
