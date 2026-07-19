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
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("Bootstrap", func() {
	var clientset *kubernetes.Clientset

	BeforeEach(func() {
		var err error
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = clientset.CoreV1().ServiceAccounts("kube-system").Delete(ctx, "inoculant", metav1.DeleteOptions{})
		_ = clientset.RbacV1().ClusterRoles().Delete(ctx, "inoculant", metav1.DeleteOptions{})
		_ = clientset.RbacV1().ClusterRoleBindings().Delete(ctx, "inoculant", metav1.DeleteOptions{})
	})

	It("scopes the generated kubeconfig to the allowed GVKs", func() {
		kubeconfigPath := filepath.Join(GinkgoT().TempDir(), "kubeconfig")

		gvks := []schema.GroupVersionKind{{Group: "", Version: "v1", Kind: "ConfigMap"}}
		Expect(inoculant.Bootstrap(ctx, cfg, gvks, kubeconfigPath)).To(Succeed())

		scopedCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())

		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: inoculant-bootstrap-allowed
  namespace: default
data:
  key: value
`), 0644)).To(Succeed())

		Expect(inoculant.Apply(ctx, dir, scopedCfg)).To(Succeed())

		got, err := clientset.CoreV1().ConfigMaps("default").Get(ctx, "inoculant-bootstrap-allowed", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["key"]).To(Equal("value"))

		disallowedDir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(disallowedDir, "ns.yaml"), []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: inoculant-bootstrap-disallowed
`), 0644)).To(Succeed())

		err = inoculant.Apply(ctx, disallowedDir, scopedCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("forbidden"))
	})
})
