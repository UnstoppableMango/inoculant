{
  container,
  runCommand,
  skopeo,
}:
runCommand "inoculant.tar" { } ''
  # --insecure-policy: source is a local nix2container image, not a remote registry.
  # No tag on the oci-archive destination: a baked-in tag (e.g. "inoculant:0.0.1")
  # gets imported by `ctr images import` as its own untagged-hash-free image name
  # alongside whatever --index-name the importer requests, and containerd/CRI can
  # resolve to that stale alias instead of the content-addressed one. Omitting the
  # tag leaves --index-name as the only name the import creates.
  ${skopeo}/bin/skopeo copy \
    --insecure-policy \
    --tmpdir=$TMPDIR \
    nix:${container} \
    oci-archive:$out
''
