package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	inoculant "github.com/unstoppablemango/inoculant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

var configMapGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
var namespaceGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}

var _ = Describe("Apply", func() {
	var clientset *kubernetes.Clientset

	BeforeEach(func() {
		var err error
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	It("applies a single YAML ConfigMap", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-single-yaml
  namespace: default
data:
  key: value
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-single-yaml", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["key"]).To(Equal("value"))
	})

	It("applies multiple resources from a multi-doc YAML", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-multi-1
  namespace: default
data:
  part: "1"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-multi-2
  namespace: default
data:
  part: "2"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		cm1, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-multi-1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cm1.Data["part"]).To(Equal("1"))

		cm2, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-multi-2", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cm2.Data["part"]).To(Equal("2"))
	})

	It("applies a JSON ConfigMap", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.json"), []byte(`{
  "apiVersion": "v1",
  "kind": "ConfigMap",
  "metadata": {
    "name": "inoculant-json",
    "namespace": "default"
  },
  "data": {
    "format": "json"
  }
}`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-json", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["format"]).To(Equal("json"))
	})

	It("applies mixed YAML and JSON files in same directory", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-mixed-yaml
  namespace: default
data:
  type: yaml
`), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{
  "apiVersion": "v1",
  "kind": "ConfigMap",
  "metadata": {
    "name": "inoculant-mixed-json",
    "namespace": "default"
  },
  "data": {
    "type": "json"
  }
}`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		yamlCM, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-mixed-yaml", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(yamlCM.Data["type"]).To(Equal("yaml"))

		jsonCM, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-mixed-json", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonCM.Data["type"]).To(Equal("json"))
	})

	It("applies resources in nested subdirectories", func() {
		dir := GinkgoT().TempDir()
		subdir := filepath.Join(dir, "subdir")
		Expect(os.MkdirAll(subdir, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(subdir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-nested
  namespace: default
data:
  depth: "1"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-nested", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["depth"]).To(Equal("1"))
	})

	It("is idempotent: applying the same directory twice succeeds", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-idempotent
  namespace: default
data:
  run: "1"
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())
		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-idempotent", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["run"]).To(Equal("1"))
	})

	It("applies resources when the given directory is a symlink", func() {
		real := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(real, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-symlink
  namespace: default
data:
  via: symlink
`), 0644)).To(Succeed())

		link := filepath.Join(GinkgoT().TempDir(), "link")
		Expect(os.Symlink(real, link)).To(Succeed())

		Expect(inoculant.Apply(ctx, link, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-symlink", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["via"]).To(Equal("symlink"))
	})

	It("fails on invalid YAML", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata: [this is not, valid: yaml
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).NotTo(Succeed())
	})

	It("fails on an unknown kind", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: NotARealKind
metadata:
  name: inoculant-unknown-kind
  namespace: default
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).NotTo(Succeed())
	})

	It("succeeds on an empty directory", func() {
		dir := GinkgoT().TempDir()

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())
	})

	It("applies a cluster-scoped resource without a namespace", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "ns.yaml"), []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-cluster-scoped
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

		got, err := clientset.CoreV1().Namespaces().Get(ctx, "inoculant-cluster-scoped", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Name).To(Equal("inoculant-cluster-scoped"))
	})

	Describe("AllowedGVKs", func() {
		It("applies a permitted GVK", func() {
			dir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-allowed-gvk
  namespace: default
data:
  allowed: "true"
`), 0644)).To(Succeed())

			opts := inoculant.Options{AllowedGVKs: []schema.GroupVersionKind{configMapGVK}}
			Expect(inoculant.Apply(ctx, dir, cfg, opts)).To(Succeed())

			got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-allowed-gvk", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Data["allowed"]).To(Equal("true"))
		})

		It("rejects a GVK not in the allowlist", func() {
			dir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(dir, "ns.yaml"), []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-rejected-ns
`), 0644)).To(Succeed())

			// Only ConfigMap allowed; Namespace must be rejected
			opts := inoculant.Options{AllowedGVKs: []schema.GroupVersionKind{configMapGVK}}
			Expect(inoculant.Apply(ctx, dir, cfg, opts)).NotTo(Succeed())
		})

		It("allows all GVKs when allowlist is empty", func() {
			dir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(dir, "ns.yaml"), []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-no-allowlist
`), 0644)).To(Succeed())

			Expect(inoculant.Apply(ctx, dir, cfg, inoculant.Options{})).To(Succeed())

			got, err := clientset.CoreV1().Namespaces().Get(ctx, "inoculant-no-allowlist", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Name).To(Equal("inoculant-no-allowlist"))
		})

		It("permits multiple GVKs in allowlist", func() {
			dir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-multi-gvk-cm
  namespace: default
data:
  x: "1"
---
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-multi-gvk-ns
`), 0644)).To(Succeed())

			opts := inoculant.Options{AllowedGVKs: []schema.GroupVersionKind{configMapGVK, namespaceGVK}}
			Expect(inoculant.Apply(ctx, dir, cfg, opts)).To(Succeed())

			_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-multi-gvk-cm", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = clientset.CoreV1().Namespaces().Get(ctx, "inoculant-multi-gvk-ns", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects on first disallowed GVK in a multi-doc file", func() {
			dir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(dir, "mixed.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-partial-ok
  namespace: default
data:
  x: "1"
---
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-partial-ns
`), 0644)).To(Succeed())

			// Namespace not in allowlist — entire apply fails
			opts := inoculant.Options{AllowedGVKs: []schema.GroupVersionKind{configMapGVK}}
			Expect(inoculant.Apply(ctx, dir, cfg, opts)).NotTo(Succeed())
		})
	})
})
