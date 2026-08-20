{
  inoculant,
  lib,
  nix2container,
  version,
}:
nix2container.buildImage {
  name = "inoculant";
  # `version` alone isn't a unique identity: it doesn't change across rebuilds
  # at the same version. Consumers that need a reference guaranteed to change
  # with content should derive their own content-addressed tag instead, the
  # way module.nix's `image` (built from a hash of `imageArchive`) does.
  tag = version;
  config.entrypoint = [
    (lib.getExe inoculant)
  ];
}
