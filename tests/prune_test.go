package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	inoculant "github.com/unstoppablemango/inoculant"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Prune", func() {
	It("deletes an object removed from the manifest set on re-apply", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-prune-keep
  namespace: default
data:
  keep: "1"
`), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-prune-remove
  namespace: default
data:
  remove: "1"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-keep", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-remove", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(os.Remove(filepath.Join(dir, "b.yaml"))).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-keep", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-remove", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("never prunes an object it did not apply", func() {
		unmanaged, err := clientset.CoreV1().ConfigMaps("default").Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "inoculant-prune-unmanaged"},
			Data:       map[string]string{"hand": "made"},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = clientset.CoreV1().ConfigMaps("default").Delete(ctx, unmanaged.Name, metav1.DeleteOptions{})
		}()

		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-prune-managed
  namespace: default
data:
  managed: "1"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, unmanaged.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("prunes nothing when re-applying an unchanged directory", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-prune-unchanged
  namespace: default
data:
  run: "1"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())
		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-unchanged", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("prunes a cluster-scoped resource removed from the manifest set", func() {
		// A Namespace won't do here: envtest runs no controller-manager, so
		// a deleted Namespace just sits in Terminating (Get still succeeds)
		// instead of actually going away. Use a ClusterRole instead, which
		// has no finalizer-driven deletion.
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cr.yaml"), []byte(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: inoculant-prune-cluster-scoped
rules: []
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err := clientset.RbacV1().ClusterRoles().Get(ctx, "inoculant-prune-cluster-scoped", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(os.Remove(filepath.Join(dir, "cr.yaml"))).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err = clientset.RbacV1().ClusterRoles().Get(ctx, "inoculant-prune-cluster-scoped", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
