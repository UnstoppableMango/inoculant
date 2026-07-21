{
  container,
  runCommand,
  skopeo,
  version,
}:
runCommand "inoculant.tar" { } ''
  # --insecure-policy: source is a local nix2container image, not a remote registry.
  ${skopeo}/bin/skopeo copy \
    --insecure-policy \
    --tmpdir=$TMPDIR \
    nix:${container} \
    oci-archive:$out:inoculant:${version}
''
