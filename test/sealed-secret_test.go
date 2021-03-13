package test

import (
	_ "embed"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/sealed-secret.yaml
var sealedSecretYAML []byte

func prepareSealedSecret() {
	It("should create a Secret to be converted for SealedSecret", func() {
		By("creating a SealedSecret")
		stdout, stderr, err := ExecAtWithInput(boot0, sealedSecretYAML, "kubeseal | kubectl apply -f -")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})
}

func testSealedSecret() {
	It("should be working", func() {
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "kubectl", "get", "secrets", "sealed-secret-test")
			if err != nil {
				return fmt.Errorf("failed to get secret: %s: %w", string(stderr), err)
			}
			return nil
		}).Should(Succeed())
	})
}
