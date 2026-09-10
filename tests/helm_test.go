package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	inoculant "github.com/unstoppablemango/inoculant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// writeChart writes a minimal chart named after filepath.Base(dir) into
// dir, so its release name (derived from the directory basename) is
// unique per test and doesn't collide with releases from other specs
// sharing the same envtest "default" namespace.
func writeChart(dir, message string) {
	name := filepath.Base(dir)
	Expect(os.MkdirAll(filepath.Join(dir, "templates"), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(`
apiVersion: v2
name: `+name+`
version: 0.1.0
`), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("message: "+message+"\n"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "templates", "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-cm
data:
  message: {{ .Values.message | quote }}
`), 0644)).To(Succeed())
}

var _ = Describe("Apply (helm)", func() {
	It("installs resources rendered from a chart directory, picking up values.yaml", func() {
		root := GinkgoT().TempDir()
		chartDir := filepath.Join(root, "chart-install")
		Expect(os.MkdirAll(chartDir, 0755)).To(Succeed())
		writeChart(chartDir, "hello")

		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "chart-install-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["message"]).To(Equal("hello"))
	})

	It("upgrades an existing release idempotently, re-rendering updated values", func() {
		root := GinkgoT().TempDir()
		chartDir := filepath.Join(root, "chart-upgrade")
		Expect(os.MkdirAll(chartDir, 0755)).To(Succeed())
		writeChart(chartDir, "hello")

		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())

		writeChart(chartDir, "goodbye")
		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "chart-upgrade-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["message"]).To(Equal("goodbye"))

		secrets, err := clientset.CoreV1().Secrets("default").List(ctx, metav1.ListOptions{
			LabelSelector: "owner=helm,name=chart-upgrade",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(secrets.Items).To(HaveLen(2), "expected two release revisions (install + upgrade)")
	})

	It("does not apply chart template files as raw manifests", func() {
		root := GinkgoT().TempDir()
		chartDir := filepath.Join(root, "chart-isolation")
		Expect(os.MkdirAll(chartDir, 0755)).To(Succeed())
		writeChart(chartDir, "hello")

		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())
	})

	It("excludes helm-managed objects from pruning", func() {
		root := GinkgoT().TempDir()
		chartDir := filepath.Join(root, "chart-prune")
		Expect(os.MkdirAll(chartDir, 0755)).To(Succeed())
		writeChart(chartDir, "hello")

		rawManifest := filepath.Join(root, "raw.yaml")
		Expect(os.WriteFile(rawManifest, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: raw-cm-helm-prune-test
data:
  key: value
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())
		_, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "raw-cm-helm-prune-test", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "chart-prune-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(os.Remove(rawManifest)).To(Succeed())
		Expect(inoculant.Apply(ctx, root, cfg)).To(Succeed())

		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "raw-cm-helm-prune-test", metav1.GetOptions{})
		Expect(err).To(HaveOccurred())
		_, err = clientset.CoreV1().ConfigMaps("default").Get(ctx, "chart-prune-cm", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})
})
