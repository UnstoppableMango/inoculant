{
  buildGoApplication,
  callPackage,
  globset,
  lib,
  version,
}:
let
  fs = lib.fileset;
in
buildGoApplication {
  pname = "inoculant";
  inherit version;

  src = fs.toSource {
    root = ../.;
    fileset = globset.lib.globs ../. [
      "go.mod"
      "go.sum"
      "**/*.go"
    ];
  };

  modules = ./gomod2nix.toml;

  # Tests use envTest
  doCheck = false;

  passthru.test = callPackage ./test.nix { };

  meta = {
    description = "A kubernetes bootstrapper";
    license = lib.licenses.mit;
    mainProgram = "inoculant";
  };
}
