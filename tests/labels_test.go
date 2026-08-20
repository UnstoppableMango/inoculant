package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	inoculant "github.com/unstoppablemango/inoculant"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Labels", func() {
	const nodeName = "inoculant-labels-test-node"

	BeforeEach(func() {
		_, err := clientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = clientset.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
	})

	It("applies labels to the target node", func() {
		labels := map[string]string{"topology.kubernetes.io/zone": "test-zone"}
		Expect(inoculant.Labels(ctx, cfg, nodeName, labels)).To(Succeed())

		got, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Labels).To(HaveKeyWithValue("topology.kubernetes.io/zone", "test-zone"))
	})

	It("is idempotent: applying the same labels twice succeeds", func() {
		labels := map[string]string{"foo": "bar"}
		Expect(inoculant.Labels(ctx, cfg, nodeName, labels)).To(Succeed())
		Expect(inoculant.Labels(ctx, cfg, nodeName, labels)).To(Succeed())

		got, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Labels).To(HaveKeyWithValue("foo", "bar"))
	})
})
