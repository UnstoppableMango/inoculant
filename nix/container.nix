{
  inoculant,
  lib,
  nix2container,
  version,
}:
nix2container.buildImage {
  name = "inoculant";
  tag = version;
  config.entrypoint = [
    (lib.getExe inoculant)
  ];
}
