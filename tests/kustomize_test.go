package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	inoculant "github.com/unstoppablemango/inoculant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Apply (kustomize)", func() {
	It("applies resources built from a kustomization directory", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  key: value
`), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte(`
namespace: default
namePrefix: inoculant-kustomize-
resources:
  - cm.yaml
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-kustomize-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["key"]).To(Equal("value"))
	})

	It("does not double-apply raw manifests inside a kustomization directory", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  key: value
`), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte(`
namespace: default
namePrefix: inoculant-nodup-
resources:
  - cm.yaml
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())

		_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "cm", metav1.GetOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("prunes objects removed from a kustomization overlay", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  key: value
`), 0644)).To(Succeed())
		kustomization := filepath.Join(dir, "kustomization.yaml")
		Expect(os.WriteFile(kustomization, []byte(`
namespace: default
namePrefix: inoculant-prune-
resources:
  - cm.yaml
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())
		_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(os.WriteFile(kustomization, []byte(`
namespace: default
namePrefix: inoculant-prune-
resources: []
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg)).To(Succeed())
		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-prune-cm", metav1.GetOptions{})
		Expect(err).To(HaveOccurred())
	})
})
