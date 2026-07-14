{
  container,
  runCommand,
  skopeo,
  version,
}:
runCommand "inoculant.tar" { } ''
  # --insecure-policy: source is a locally-built nix2container image
  # (nix:${container}), not a remote registry, so there's no signature
  # policy to verify against.
  ${skopeo}/bin/skopeo copy \
    --insecure-policy \
    --tmpdir=$TMPDIR \
    nix:${container} \
    oci-archive:$out:inoculant:${version}
''
