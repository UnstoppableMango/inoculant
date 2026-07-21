{
  inputs,
  version,
}:
{
  pkgs,
  lib,
  config,
  ...
}:
let
  inherit (inputs) globset;
  inherit (pkgs.stdenv.hostPlatform) system;
  inherit (inputs.nix2container.packages.${system})
    nix2container
    skopeo-nix2container
    ;

  inherit (inputs.gomod2nix.legacyPackages.${system})
    buildGoApplication
    ;

  top = config.services.kubernetes;
  cfg = top.inoculant;

  # Real directory of copies, not symlinks, since the pod bind-mounts only this directory.
  manifestsDrv = pkgs.runCommand "inoculant-manifests" { } (
    ''
      mkdir -p "$out"
    ''
    + lib.concatStrings (
      lib.mapAttrsToList (
        name: manifest:
        let
          # Multiple manifests under one name are written as consecutive JSON documents in one file; internal/manifest.Parse reads them as a stream.
          content =
            if lib.isList manifest then
              lib.concatMapStringsSep "\n" builtins.toJSON manifest
            else
              builtins.toJSON manifest;
        in
        ''
          install -Dm444 ${pkgs.writeText "${name}.json" content} "$out/"${lib.escapeShellArg "${name}.json"}
        ''
      ) cfg.manifests
    )
    + lib.concatMapStrings (src: ''
      cp -r --no-preserve=mode,ownership ${src} "$out/$(basename ${src})"
      chmod -R a+rX "$out/$(basename ${src})"
    '') cfg.manifestFiles
  );

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
    ) cfg.manifests
  );

  allAllowedGVKs = lib.unique (derivedGVKs ++ cfg.additionalAllowedGVKs);

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
in
{
  options.services.kubernetes.inoculant = {
    enable = lib.mkEnableOption "A kubernetes bootstrapper";

    # Built from this module's own `pkgs` arg rather than the flake's package, so the module stays import-able on any nixpkgs pin.
    pkg = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./inoculant.nix {
        inherit globset version buildGoApplication;
      };
    };

    # Same self-containment reasoning as `pkg` above: rebuilt from `cfg.pkg`.
    imageArchive = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./tarball.nix {
        inherit (cfg) skopeo;
        inherit version;

        container = pkgs.callPackage ./container.nix {
          inoculant = cfg.pkg;
          inherit nix2container version;
        };
      };
    };

    skopeo = lib.mkOption {
      type = lib.types.package;
      default = skopeo-nix2container;
    };

    manifestsDirectory = lib.mkOption {
      type = lib.types.externalPath;
      default = "/etc/inoculant/manifests";
      description = "Host directory containing static manifests for inoculant to apply.";
    };

    # Same shape as services.kubernetes.addonManager.addons. Keys become filenames, so must be plain names.
    manifests = lib.mkOption {
      type = lib.types.attrsOf (lib.types.either lib.types.attrs (lib.types.listOf lib.types.attrs));
      default = { };
      description = "Static manifests seeded into manifestsDirectory for inoculant to apply. Each entry is either a single manifest or a list of manifests sharing one output file.";
    };

    manifestFiles = lib.mkOption {
      type = lib.types.listOf lib.types.path;
      default = [ ];
      description = "Extra manifest files or directories copied into manifestsDirectory verbatim, alongside `manifests`.";
    };

    # GVKs that can't be auto-derived at eval time (e.g. from YAML manifestFiles), merged with the derived set.
    additionalAllowedGVKs = lib.mkOption {
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            group = lib.mkOption {
              type = lib.types.str;
              default = "";
              description = "API group (empty string for core API).";
            };
            ver = lib.mkOption {
              type = lib.types.str;
              description = "API version (e.g. v1, v1beta1).";
            };
            kind = lib.mkOption {
              type = lib.types.str;
              description = "Resource kind (e.g. Deployment, ConfigMap).";
            };
          };
        }
      );
      default = [ ];
      description = "Extra GVKs inoculant is permitted to apply, beyond those auto-derived from `manifests`. This is the only way to grant permissions for resources coming from `manifestFiles`, since Nix cannot introspect those files' contents at eval time.";
    };
  };

  config = lib.mkIf cfg.enable (
    let
      image = "docker.io/library/inoculant:${version}";

      # top.pki.clusterAdminKubeconfig isn't an exposed option, so rebuild it the same way pki.nix does internally.
      # Only the bootstrap init container uses this; the main container uses the scoped token it writes.
      kubeconfigFile = top.lib.mkKubeConfig "cluster-admin" {
        server = top.apiserverAddress;
        certFile = top.pki.certs.clusterAdmin.cert;
        keyFile = top.pki.certs.clusterAdmin.key;
      };
    in
    {
      # TODO: reimports the archive on every kubelet restart, not just first boot.
      systemd.services.kubelet.preStart = lib.mkAfter ''
        ${pkgs.containerd}/bin/ctr -n k8s.io images import --index-name ${image} ${cfg.imageArchive}
      '';

      systemd.tmpfiles.rules = [
        "L+ ${cfg.manifestsDirectory} - - - - ${manifestsDrv}"
      ];

      services.kubernetes.kubelet.manifests.inoculant = {
        apiVersion = "v1";
        kind = "Pod";
        metadata = {
          name = "inoculant";
          namespace = "kube-system";
        };
        spec = {
          restartPolicy = "OnFailure";
          hostNetwork = true;
          dnsPolicy = "Default";

          # Phase 1: create scoped RBAC + write token kubeconfig to emptyDir.
          initContainers = [
            {
              name = "inoculant-bootstrap";
              image = image;
              args = [
                "--kubeconfig"
                "/etc/inoculant/cluster-admin.kubeconfig"
                "bootstrap"
                "--output"
                "/scoped-kubeconfig/kubeconfig"
              ]
              ++ allowArgs;
              volumeMounts = [
                {
                  name = "cluster-admin-kubeconfig";
                  mountPath = "/etc/inoculant/cluster-admin.kubeconfig";
                  readOnly = true;
                }
                {
                  name = "ca-cert";
                  mountPath = top.caFile;
                  readOnly = true;
                }
                {
                  name = "client-cert";
                  mountPath = top.pki.certs.clusterAdmin.cert;
                  readOnly = true;
                }
                {
                  name = "client-key";
                  mountPath = top.pki.certs.clusterAdmin.key;
                  readOnly = true;
                }
                {
                  name = "scoped-kubeconfig";
                  mountPath = "/scoped-kubeconfig";
                }
              ];
            }
          ];

          # Phase 2: apply manifests using the scoped token kubeconfig.
          containers = [
            {
              name = "inoculant";
              image = image;
              args = [
                "--kubeconfig"
                "/scoped-kubeconfig/kubeconfig"
                "apply"
                "/etc/inoculant/manifests"
              ];
              volumeMounts = [
                {
                  name = "scoped-kubeconfig";
                  mountPath = "/scoped-kubeconfig";
                  readOnly = true;
                }
                {
                  name = "manifests";
                  mountPath = "/etc/inoculant/manifests";
                  readOnly = true;
                }
              ];
            }
          ];

          volumes = [
            {
              name = "cluster-admin-kubeconfig";
              hostPath.path = kubeconfigFile;
            }
            {
              name = "ca-cert";
              hostPath.path = top.caFile;
            }
            {
              name = "client-cert";
              hostPath.path = top.pki.certs.clusterAdmin.cert;
            }
            {
              name = "client-key";
              hostPath.path = top.pki.certs.clusterAdmin.key;
            }
            {
              name = "scoped-kubeconfig";
              emptyDir = { };
            }
            {
              name = "manifests";
              hostPath.path = cfg.manifestsDirectory;
            }
          ];
        };
      };
    }
  );
}
