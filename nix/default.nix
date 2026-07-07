{
  buildGoApplication,
  callPackage,
  lib,
  ginkgo,
  version,
}:
buildGoApplication {
  pname = "inoculant";
  inherit version;

  src = lib.cleanSource ../.;
  modules = ./gomod2nix.toml;

  passthru.test = callPackage ./test.nix { };

  nativeCheckInputs = [ ginkgo ];

  doCheck = false;
  checkPhase = ''
    ginkgo run ./...
  '';
}
