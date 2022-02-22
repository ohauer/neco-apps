package test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testHubble() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("hubble-ui", "kube-system", 1)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("hubble-observer", "kube-system", 1)
		}).Should(Succeed())
	})

	It("should serve hubble web ui", func() {
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "curl", "http://hubble-ui.kube-system.svc:80")
			if err != nil {
				return fmt.Errorf("unable to curl http://hubble-ui.kube-system.svc:80, stderr: %s, err: %v", stderr, err)
			}

			return nil
		}).Should(Succeed())
	})
}
