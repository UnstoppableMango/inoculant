{
  pkgs,
  lib,
  manifests,
  manifestFiles,
}:
# Real directory of copies, not symlinks, since the pod bind-mounts only this directory.
pkgs.runCommand "inoculant-manifests" { } (
  ''
    mkdir -p "$out"
  ''
  + lib.concatStrings (
    lib.mapAttrsToList (
      name: manifest:
      let
        # Keys become filenames under $out; a "/" would write into a subpath (or escape $out via "..").
        checkedName = lib.throwIfNot (
          !lib.hasInfix "/" name
        ) "services.kubernetes.inoculant.manifests: key \"${name}\" must be a plain name, not a path" name;
        # Multiple manifests under one name are written as consecutive JSON documents in one file; internal/manifest.Parse reads them as a stream.
        content =
          if lib.isList manifest then
            lib.concatMapStringsSep "\n" builtins.toJSON manifest
          else
            builtins.toJSON manifest;
      in
      ''
        install -Dm444 ${pkgs.writeText "${checkedName}.json" content} "$out/"${lib.escapeShellArg "${checkedName}.json"}
      ''
    ) manifests
  )
  + lib.concatMapStrings (src: ''
    cp -r --no-preserve=mode,ownership ${src} "$out/$(basename ${src})"
    chmod -R a+rX "$out/$(basename ${src})"
  '') manifestFiles
)
