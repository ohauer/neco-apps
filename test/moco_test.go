package test

import (
	_ "embed"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/moco.yaml
var mocoYAML []byte

func prepareMoco() {
	It("should deploy mysqlcluster", func() {
		By("preparing namespace")
		createNamespaceIfNotExists("test-moco", false)

		By("creating mysqlcluster")
		_, stderr, err := ExecAtWithInput(boot0, mocoYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testMoco() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("moco-controller", "moco-system", 2)
		}).Should(Succeed())
	})

	It("should make mysqlcluster ready", func() {
		By("waiting mysqlcluster is ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=test-moco", "get", "mysqlcluster/test", "-o", `"jsonpath={.status.conditions[?(@.type=='Healthy')].status}"`)
			if err != nil {
				return fmt.Errorf("mysqlcluter is not healthy: %s: %w", stderr, err)
			}

			if string(stdout) != "True" {
				return errors.New("MySQLCluster is not ready")
			}
			return nil
		}).Should(Succeed())

		By("running kubectl moco mysql")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "moco", "-n", "test-moco", "mysql", "-u", "moco-admin", "test", "--", "--version")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(string(stdout)).Should(ContainSubstring("mysql  Ver 8"))
	})
}
