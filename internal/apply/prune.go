package apply

import (
	"context"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// prune deletes every object carrying the managed-by label that is not in
// desired. It discovers candidate resource types via the API server rather
// than tracking previously-applied state, so it can find objects whose
// entire manifest (or Kind) was removed from the manifest set.
//
// Errors are logged and swallowed per resource type rather than propagated:
// a bootstrap-scoped token only has RBAC for the GVKs currently allowed
// (see GOALS.md), so Forbidden errors for GVKs no longer present in the
// manifest set are expected, not exceptional. One resource type failing to
// list or an individual delete failing must never fail the overall Apply.
func (a *Applier) prune(ctx context.Context, desired map[objectKey]struct{}) error {
	lists, err := a.c.Clientset.Discovery().ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		klog.ErrorS(err, "discover API resources for pruning")
		return nil
	}
	if err != nil {
		klog.V(1).InfoS("partial API resource discovery", "err", err)
	}

	selector := metav1.ListOptions{LabelSelector: managedByLabel + "=" + managedByValue}

	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			klog.V(1).InfoS("skipping unparsable group version", "groupVersion", list.GroupVersion)
			continue
		}

		for _, res := range list.APIResources {
			if strings.Contains(res.Name, "/") {
				continue // subresource
			}
			if !hasVerbs(res.Verbs, "list", "delete") {
				continue
			}

			gvr := gv.WithResource(res.Name)
			a.pruneResource(ctx, gvr, res.Namespaced, selector, desired)
		}
	}

	return nil
}

func (a *Applier) pruneResource(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	namespaced bool,
	selector metav1.ListOptions,
	desired map[objectKey]struct{},
) {
	var ri dynamic.ResourceInterface
	if namespaced {
		ri = a.c.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll)
	} else {
		ri = a.c.Dynamic.Resource(gvr)
	}

	list, err := ri.List(ctx, selector)
	if err != nil {
		if apierrors.IsForbidden(err) {
			klog.V(1).InfoS("no permission to list resource type for pruning, skipping", "resource", gvr)
		} else if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "list resources for pruning", "resource", gvr)
		}
		return
	}

	for _, item := range list.Items {
		key := objectKey{GroupVersionResource: gvr, Namespace: item.GetNamespace(), Name: item.GetName()}
		if _, ok := desired[key]; ok {
			continue
		}

		klog.InfoS("pruning object", "resource", gvr, "namespace", item.GetNamespace(), "name", item.GetName())

		var deleteErr error
		if namespaced {
			deleteErr = a.c.Dynamic.Resource(gvr).Namespace(item.GetNamespace()).Delete(ctx, item.GetName(), metav1.DeleteOptions{})
		} else {
			deleteErr = a.c.Dynamic.Resource(gvr).Delete(ctx, item.GetName(), metav1.DeleteOptions{})
		}
		if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
			klog.ErrorS(deleteErr, "prune object", "resource", gvr, "namespace", item.GetNamespace(), "name", item.GetName())
		}
	}
}

func hasVerbs(verbs metav1.Verbs, want ...string) bool {
	for _, w := range want {
		if !slices.Contains(verbs, w) {
			return false
		}
	}
	return true
}
