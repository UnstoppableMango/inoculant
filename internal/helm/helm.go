// Package helm installs and upgrades Helm chart releases found in local
// chart directories during the apply walk, using Helm's own Secret-backed
// release storage rather than inoculant's server-side-apply/prune
// mechanism.
package helm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/unstoppablemango/inoculant/internal/client"
)

// chartMarkerFile is the file inoculant checks for during the directory
// walk to decide whether dir is a Helm chart root, mirroring how
// kustomize.IsRoot checks for a kustomization file.
const chartMarkerFile = "Chart.yaml"

// IsRoot reports whether dir is a Helm chart directory (contains a
// Chart.yaml at its root).
func IsRoot(fSys filesys.FileSystem, dir string) bool {
	return fSys.Exists(filepath.Join(dir, chartMarkerFile))
}

// ReleaseName derives a Helm release name from a chart directory's base
// name. This is the sole naming convention for chart directories: there
// are no CLI flags or in-chart overrides for release name, matching how
// kustomize overlays need no separate name either.
func ReleaseName(dir string) string {
	return filepath.Base(filepath.Clean(dir))
}

// Install installs or upgrades the chart rooted at dir as a Helm release
// named after dir's base name, in namespace ns, against the cluster
// described by c. Chart-default values, including a values.yaml at the
// chart root, are loaded automatically by the chart loader.
func Install(ctx context.Context, c *client.Client, dir, ns string) (*release.Release, error) {
	name := ReleaseName(dir)

	chrt, err := loader.LoadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("load chart %s: %w", dir, err)
	}

	cfg, err := newConfiguration(c, ns)
	if err != nil {
		return nil, fmt.Errorf("init helm configuration: %w", err)
	}

	hist := action.NewHistory(cfg)
	hist.Max = 1
	if _, err := hist.Run(name); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return install(ctx, cfg, name, ns, chrt)
		}
		return nil, fmt.Errorf("check history for release %s: %w", name, err)
	}

	return upgrade(ctx, cfg, name, chrt)
}

func install(ctx context.Context, cfg *action.Configuration, name, ns string, chrt *chart.Chart) (*release.Release, error) {
	i := action.NewInstall(cfg)
	i.ReleaseName = name
	i.Namespace = ns
	i.CreateNamespace = true

	klog.InfoS("installing helm release", "name", name, "namespace", ns)
	rel, err := i.RunWithContext(ctx, chrt, nil)
	if err != nil {
		return nil, fmt.Errorf("install release %s: %w", name, err)
	}
	return rel, nil
}

func upgrade(ctx context.Context, cfg *action.Configuration, name string, chrt *chart.Chart) (*release.Release, error) {
	u := action.NewUpgrade(cfg)

	klog.InfoS("upgrading helm release", "name", name)
	rel, err := u.RunWithContext(ctx, name, chrt, nil)
	if err != nil {
		return nil, fmt.Errorf("upgrade release %s: %w", name, err)
	}
	return rel, nil
}

func newConfiguration(c *client.Client, ns string) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	getter := &restClientGetter{c: c, namespace: ns}
	debugLog := func(format string, v ...interface{}) {
		klog.V(1).InfoS("helm", "msg", fmt.Sprintf(format, v...))
	}
	if err := cfg.Init(getter, ns, "secret", debugLog); err != nil {
		return nil, err
	}
	return cfg, nil
}
