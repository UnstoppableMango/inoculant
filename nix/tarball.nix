{
  container,
  runCommand,
  skopeo,
  version,
}:
runCommand "inoculant.tar" { } ''
  ${skopeo}/bin/skopeo copy \
    --insecure-policy \
    --tmpdir=$TMPDIR \
    nix:${container} \
    oci-archive:$out:inoculant:${version}
''
