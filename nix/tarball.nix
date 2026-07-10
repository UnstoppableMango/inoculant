{
  container,
  runCommand,
  skopeo,
  version,
}:
runCommand "inoculant.tar" { } ''
  ${skopeo}/bin/skopeo copy \
    nix:${container} \
    oci-archive:$out:${version}
''
