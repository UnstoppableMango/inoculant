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

  manifestsDrv = import ./manifests.nix {
    inherit pkgs lib;
    inherit (cfg) manifests manifestFiles;
  };

  inherit
    (import ./gvk.nix {
      inherit lib;
      inherit (cfg) manifests additionalAllowedGVKs nodeLabels;
    })
    allowArgs
    labelArgs
    ;
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

    # Same shape as services.kubernetes.addonManager.addons. Keys become filenames, so they must be plain names.
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

    nodeLabels = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      description = "Labels to apply to this node via a `label-node` init container, filling the gap left by kubelet's deprecated --node-labels.";
    };

    clusterAdmin = lib.mkOption {
      description = ''
        Client certificate with cluster-admin privileges, used by the bootstrap
        init container to mint scoped RBAC and a token kubeconfig.

        Defaults to the standard easyCerts-managed clusterAdmin cert. Set this
        explicitly on setups that don't populate `pki.certs.clusterAdmin` (e.g.
        `easyCerts = false`): that option is also unconditionally defined by
        `services.kubernetes.apiserver` whenever it's enabled, and since
        `pki.certs` merges same-priority definitions with a plain `//`, a
        second definition of the same key doesn't reliably win against it.
      '';
      default = {
        inherit (top.pki.certs.clusterAdmin) cert key;
      };
      defaultText = lib.literalExpression "config.services.kubernetes.pki.certs.clusterAdmin";
      type = lib.types.submodule {
        options = {
          cert = lib.mkOption {
            type = lib.types.path;
            description = "Path to the client certificate.";
          };
          key = lib.mkOption {
            type = lib.types.path;
            description = "Path to the private key matching `cert`.";
          };
        };
      };
    };
  };

  config = lib.mkIf cfg.enable (
    let
      # Content-address the image reference so a rebuilt binary (even at the same
      # `version`) yields a new reference. The `image:` field is part of the static
      # pod manifest, so a changed reference makes kubelet tear down and recreate
      # the pod, re-running bootstrap + apply. It also guarantees the freshly built
      # image is the one imported/served rather than a stale same-tag image, since
      # `ctr images import --index-name` below names the image by this reference.
      imageHash = builtins.substring 0 16 (builtins.hashString "sha256" "${cfg.imageArchive}");
      image = "docker.io/library/inoculant:${version}-${imageHash}";

      # Content hash of the applied manifest set. Embedded as a pod annotation so
      # that editing any manifest changes the static pod manifest, causing kubelet
      # to recreate the pod and re-run apply. manifestsDrv's store path already
      # incorporates the content of every manifest.
      manifestsHash = builtins.hashString "sha256" "${manifestsDrv}";

      # top.pki.clusterAdminKubeconfig isn't an exposed option, so rebuild it the same way pki.nix does internally.
      # Only the bootstrap init container uses this; the main container uses the scoped token it writes.
      kubeconfigFile = top.lib.mkKubeConfig "cluster-admin" {
        server = top.apiserverAddress;
        certFile = cfg.clusterAdmin.cert;
        keyFile = cfg.clusterAdmin.key;
      };
    in
    {
      # image is content-addressed (see imageHash above), so an existing image
      # under this exact tag is guaranteed to already be the right content.
      # Skip the import on kubelet restarts where nothing changed; only a
      # rebuilt binary (new imageHash, new tag) triggers a real import.
      systemd.services.kubelet.preStart = lib.mkAfter ''
        if ! ${pkgs.containerd}/bin/ctr -n k8s.io images ls -q "name==${image}" | grep -q .; then
          ${pkgs.containerd}/bin/ctr -n k8s.io images import --index-name ${image} ${cfg.imageArchive}
        fi
      '';

      # Populate manifestsDirectory via environment.etc when possible, rather than
      # systemd.tmpfiles.rules. environment.etc entries (including the static pod
      # manifest below) are written by system.activationScripts.etc, which
      # switch-to-configuration runs synchronously and first. systemd.tmpfiles
      # rules are only applied later, when sysinit-reactivation.target is
      # restarted, strictly after that activation script (and the new
      # manifests-hash annotation it writes) is already visible to kubelet. That
      # ordering is backwards for re-apply: kubelet could recreate the pod before
      # the manifestsDirectory symlink flips to the new content, re-applying
      # stale manifests. Landing both writes in the same activation phase closes
      # that window.
      environment.etc = lib.optionalAttrs (lib.hasPrefix "/etc/" cfg.manifestsDirectory) {
        ${lib.removePrefix "/etc/" cfg.manifestsDirectory}.source = manifestsDrv;
      };

      # Fallback for manifestsDirectory outside /etc, where environment.etc can't
      # reach. Still carries the ordering caveat above: prefer the default
      # /etc-rooted path to avoid it.
      systemd.tmpfiles.rules = lib.optional (!lib.hasPrefix "/etc/" cfg.manifestsDirectory) (
        "L+ ${cfg.manifestsDirectory} - - - - ${manifestsDrv}"
      );

      services.kubernetes.kubelet.manifests.inoculant = {
        apiVersion = "v1";
        kind = "Pod";
        metadata = {
          name = "inoculant";
          namespace = "kube-system";
          # Re-apply trigger: kubelet derives a static pod's identity from a hash
          # of its manifest file, so any change here recreates the pod. This
          # annotation changes whenever the manifest set changes, propagating
          # manifest edits on the next `nixos-rebuild switch`. Image changes are
          # propagated separately via the content-addressed `image` reference.
          annotations."inoculant.unmango.dev/manifests-hash" = manifestsHash;
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
                  mountPath = cfg.clusterAdmin.cert;
                  readOnly = true;
                }
                {
                  name = "client-key";
                  mountPath = cfg.clusterAdmin.key;
                  readOnly = true;
                }
                {
                  name = "scoped-kubeconfig";
                  mountPath = "/scoped-kubeconfig";
                }
              ];
            }
          ];

          # Phase 2: apply manifests / label the node using the scoped token kubeconfig.
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
          ]
          ++ lib.optional (cfg.nodeLabels != { }) {
            name = "inoculant-label-node";
            image = image;
            env = [
              {
                name = "NODE_NAME";
                valueFrom.fieldRef.fieldPath = "spec.nodeName";
              }
            ];
            args = [
              "--kubeconfig"
              "/scoped-kubeconfig/kubeconfig"
              "label-node"
            ]
            ++ labelArgs;
            volumeMounts = [
              {
                name = "scoped-kubeconfig";
                mountPath = "/scoped-kubeconfig";
                readOnly = true;
              }
            ];
          };

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
              hostPath.path = cfg.clusterAdmin.cert;
            }
            {
              name = "client-key";
              hostPath.path = cfg.clusterAdmin.key;
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
