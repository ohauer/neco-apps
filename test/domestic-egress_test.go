package test

import (
	_ "embed"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/domestic-egress.yaml
var domesticEgressYAML []byte

func prepareDomesticEgress() {
	It("should create ubuntu pod on sandbox and team=network ns", func() {
		stdout, stderr, err := ExecAtWithInput(boot0, domesticEgressYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})
}

func testDomesticEgress() {
	It("should deploy coil egress successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("network-nat", "domestic-egress", 2)
		}).Should(Succeed())

		By("should not access to Neco switch from Pods in namespaces that are not team=network")
		ns := "dctest"
		var podName string
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", ns, "get", "pods", "-l", "app=ubuntu-domestic-egress-test", "-o", "json")
			if err != nil {
				return fmt.Errorf("stderr: %s: %w", string(stderr), err)
			}
			podList := &corev1.PodList{}
			if err := json.Unmarshal(stdout, podList); err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList length is not 1: %d", len(podList.Items))
			}
			podName = podList.Items[0].Name
			return nil
		}).Should(Succeed())

		// 10.72.2.0 is neco switch
		_, _, err := ExecAt(boot0, "kubectl", "-n", ns, "exec", podName, "--", "ping", "-c", "1", "-W", "3", "10.72.2.0")
		Expect(err).To(HaveOccurred())

		By("should access to Neco switch from Pods in namespaces that are team=network")
		ns = "dev-network"
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", ns, "get", "pods", "-l", "app=ubuntu-domestic-egress-test", "-o", "json")
			if err != nil {
				return fmt.Errorf("stderr: %s: %w", string(stderr), err)
			}
			podList := &corev1.PodList{}
			if err := json.Unmarshal(stdout, podList); err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList length is not 1: %d", len(podList.Items))
			}
			podName := podList.Items[0].Name
			// 10.72.2.0 is neco switch
			stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", ns, "exec", podName, "--", "ping", "-c", "1", "-W", "3", "10.72.2.0")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}
