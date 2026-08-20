{
  lib,
  manifests,
  additionalAllowedGVKs,
  nodeLabels,
}:
let
  # Derive allowed GVKs from cfg.manifests, an attrset of manifest or list-of-manifest mirroring addonManager.addons.
  derivedGVKs = lib.flatten (
    lib.mapAttrsToList (
      name: manifest:
      let
        items = if lib.isList manifest then manifest else [ manifest ];
      in
      map (
        item:
        let
          apiVersion = item.apiVersion or (throw "manifest ${name}: missing apiVersion");
          kind = item.kind or (throw "manifest ${name}: missing kind");
          parts = lib.splitString "/" apiVersion;
          group = if lib.length parts == 2 then lib.head parts else "";
          ver = lib.last parts;
        in
        if
          (lib.length parts != 1 && lib.length parts != 2)
          || ver == ""
          || (lib.length parts == 2 && group == "")
        then
          throw "manifest ${name}: invalid apiVersion ${apiVersion}, want VERSION or GROUP/VERSION with non-empty parts"
        else
          {
            inherit group ver kind;
          }
      ) items
    ) manifests
  );

  allAllowedGVKs = lib.unique (
    derivedGVKs
    ++ additionalAllowedGVKs
    ++ lib.optional (nodeLabels != { }) {
      group = "";
      ver = "v1";
      kind = "Node";
    }
  );

  # --allow GROUP/VERSION/KIND args for the bootstrap init container.
  # Empty group uses the empty string (core API).
  allowArgs = lib.concatMap (
    {
      group,
      ver,
      kind,
    }:
    [
      "--allow"
      "${group}/${ver}/${kind}"
    ]
  ) allAllowedGVKs;

  # --label key=value args for the label-node init container.
  labelArgs = lib.concatLists (
    lib.mapAttrsToList (name: value: [
      "--label"
      "${name}=${value}"
    ]) nodeLabels
  );
in
{
  inherit allowArgs labelArgs;
}
