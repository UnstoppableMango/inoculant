package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("Apply", func() {
	It("applies a ConfigMap to the cluster", func() {
		clientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inoculant-test",
				Namespace: "default",
			},
			Data: map[string]string{"key": "value"},
		}
		_, err = clientset.CoreV1().ConfigMaps("default").
			Create(ctx, cm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		got, err := clientset.CoreV1().ConfigMaps("default").
			Get(ctx, "inoculant-test", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Data["key"]).To(Equal("value"))
	})
})
