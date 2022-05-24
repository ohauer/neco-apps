package test

import (
	_ "embed"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	testTenantNamespace  = "dev-tenant-netpol"
	testTenantNamespace2 = "dev-tenant-netpol2"
	testTenantTeam       = "neco-guests"
	testRootNamespace    = "dev-guests"
)

var (
	//go:embed testdata/tenant-network-policy.yaml
	tenantNetworkPolicyYAML []byte
	//go:embed testdata/tenant-networkpolicy-bmc.yaml
	tenantNetworkPolicyBmcYAML []byte
	//go:embed testdata/tenant-networkpolicy-node.yaml
	tenantNetworkPolicyNodeYAML []byte
	//go:embed testdata/tenant-networkpolicy-node-entity.yaml
	tenantNetworkPolicyNodeEntityYAML []byte
)

func prepareTenantNetworkPolicy() {
	It("should prepare test pods in test namespaces", func() {
		By("preparing namespaces")
		ExecSafeAt(boot0, "kubectl", "accurate", "sub", "create", testTenantNamespace, testRootNamespace)
		ExecSafeAt(boot0, "kubectl", "accurate", "sub", "create", testTenantNamespace2, testRootNamespace)
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "namespace", testTenantNamespace)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "namespace", testTenantNamespace2)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("opting namespaces into network policies")
		ExecSafeAt(boot0, "kubectl", "annotate", "namespace", testTenantNamespace, "tenet.cybozu.io/network-policy-template=allow-same-namespace-ingress")
		ExecSafeAt(boot0, "kubectl", "annotate", "namespace", testTenantNamespace2, "tenet.cybozu.io/network-policy-template=allow-same-team-ingress")

		By("deploying resources")
		_, stderr, err := ExecAtWithInput(boot0, tenantNetworkPolicyYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testTenantNetworkPolicy() {
	It("should pass/block ingress accordingly", func() {
		By("waiting for testhttpd pods")
		Eventually(func() error {
			if err := checkDeploymentReplicas("testhttpd", testTenantNamespace, 2); err != nil {
				return err
			}
			return checkDeploymentReplicas("testhttpd", testTenantNamespace2, 2)
		}).Should(Succeed())

		By("waiting for ubuntu pod")
		Eventually(func() error {
			checkUbuntuPod := func(ns string) error {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", ns, "exec", "ubuntu", "--", "date")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return nil
			}
			if err := checkUbuntuPod("default"); err != nil {
				return err
			}
			if err := checkUbuntuPod(testTenantNamespace); err != nil {
				return err
			}
			return checkUbuntuPod(testTenantNamespace2)
		}).Should(Succeed())

		testConnectivity := func(srcName, srcNamespace, destName, destNamespace string) error {
			_, _, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", srcNamespace, "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", fmt.Sprintf("%s.%s.svc", destName, destNamespace), "80", "-e", "X")
			return err
		}

		By("ensuring ingress for same team is allowed")
		Eventually(func() error {
			return testConnectivity("ubuntu", testTenantNamespace, "testhttpd", testTenantNamespace2)
		}).ShouldNot(HaveOccurred())

		// TODO: actually verify non-connectivity once the temporary
		// tenant-ingress-cluster-allow policy is removed
		// (once tenants have migrated to tenet)
		By("ensuring ingress from different namespaces is blocked")
		Eventually(func() error {
			if err := testConnectivity("ubuntu", testTenantNamespace2, "testhttpd", testTenantNamespace); err != nil {
				return err
			}
			if err := testConnectivity("ubuntu", "default", "testhttpd", testTenantNamespace); err != nil {
				return err
			}
			return testConnectivity("ubuntu", "default", "testhttpd", testTenantNamespace2)
		}).ShouldNot(HaveOccurred())
	})

	It("should prevent tenants from specifying forbidden IPs in their network policies", func() {
		By("attempting to apply policy with forbidden IP")
		_, _, err := ExecAtWithInput(boot0, tenantNetworkPolicyNodeYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		_, _, err = ExecAtWithInput(boot0, tenantNetworkPolicyBmcYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())

		By("attempting to apply policy with forbidden entity")
		_, _, err = ExecAtWithInput(boot0, tenantNetworkPolicyNodeEntityYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
	})
}
