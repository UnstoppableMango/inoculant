{
  buildGoApplication,
  globset,
  lib,
  version,
}:
buildGoApplication {
  pname = "inoculant";
  inherit version;

  src = lib.fileset.toSource {
    root = ../.;
    fileset = globset.lib.globs ../. [
      "go.mod"
      "go.sum"
      "**/*.go"
    ];
  };

  modules = ./gomod2nix.toml;

  # Tests use envtest
  doCheck = false;

  meta = {
    description = "A kubernetes bootstrapper";
    license = lib.licenses.mit;
    mainProgram = "inoculant";
  };
}
