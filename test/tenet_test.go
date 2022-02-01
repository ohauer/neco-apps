package test

import (
	_ "embed"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	tenetNecoNamespace        = "dev-tenet-neco"
	tenetTenantRootNamespace  = "dev-tenet-tenant-root"
	tenetTenantChildNamespace = "dev-tenet-tenant-child"
	tenetTemplateName         = "allow-intra-namespace-egress"
)

var (
	//go:embed testdata/tenet.yaml
	tenetYAML []byte

	//go:embed testdata/tenet-privileged-ciliumnetworkpolicy.yaml
	tenetPrivilegedCiliumNetworkPolicyYAML []byte

	//go:embed testdata/tenet-subnamespace.yaml
	tenetSubnamespaceYAML []byte
)

func prepareTenet() {
	It("should deploy networkpolicytemplate and networkpolicyadmissionrule", func() {
		By("applying resources")
		_, stderr, err := ExecAtWithInput(boot0, tenetYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
	It("should setup namespaces for tenet", func() {
		By("creating neco namespace")
		createNamespaceIfNotExists(tenetNecoNamespace, false)
		ExecSafeAt(boot0, "kubectl", "label", "namespace", tenetNecoNamespace, "team=neco")
		By("creating tenant namespace")
		createNamespaceIfNotExists(tenetTenantRootNamespace, false)
		ExecSafeAt(boot0, "kubectl", "annotate", "namespace", tenetTenantRootNamespace, fmt.Sprintf("tenet.cybozu.io/network-policy-template=%s", tenetTemplateName))
		By("setting up namespace as accurate root")
		ExecSafeAt(boot0, "kubectl", "label", "namespace", tenetTenantRootNamespace, "accurate.cybozu.com/type=root")
		By("creating child namespace")
		_ = ExecSafeAtWithInput(boot0, tenetSubnamespaceYAML, "kubectl", "apply", "-f", "-")
	})
}

func testTenet() {
	var getCiliumNetworkPolicyInNamespace = func(ns, name string) error {
		_, stderr, err := ExecAt(boot0, "kubectl", "-n", ns, "get", "ciliumnetworkpolicy", name)
		if err != nil {
			return fmt.Errorf("failed to retrieve ciliumnetworkpolicy: %s, %w", string(stderr), err)
		}
		return nil
	}

	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("tenet-controller-manager", "tenet-system", 2)
		}).Should(Succeed())
	})

	It("should create ciliumnetworkpolicies", func() {
		Eventually(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantRootNamespace, tenetTemplateName)
		}).Should(Succeed())
	})

	It("should propagate template to child namespaces", func() {

		Eventually(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantChildNamespace, tenetTemplateName)
		}).Should(Succeed())
	})

	It("should cleanup namespaces when opting-out", func() {
		By("removing annotation")
		ExecSafeAt(boot0, "kubectl", "annotate", "namespace", tenetTenantRootNamespace, "tenet.cybozu.io/network-policy-template-")
		ExecSafeAt(boot0, "kubectl", "annotate", "namespace", tenetTenantChildNamespace, "tenet.cybozu.io/network-policy-template-")
		Eventually(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantRootNamespace, tenetTemplateName)
		}).ShouldNot(Succeed())
		Eventually(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantChildNamespace, tenetTemplateName)
		}).ShouldNot(Succeed())

		Consistently(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantRootNamespace, tenetTemplateName)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return getCiliumNetworkPolicyInNamespace(tenetTenantChildNamespace, tenetTemplateName)
		}).ShouldNot(Succeed())
	})

	It("should apply networkpolicyadmissionrules", func() {
		By("creating privileged policy in restricted namespace")
		_, _, err := ExecAtWithInput(boot0, tenetPrivilegedCiliumNetworkPolicyYAML, "kubectl", "-n", tenetTenantRootNamespace, "apply", "-f", "-")
		Expect(err).To(HaveOccurred())

		By("creating privileged policy in neco namespace")
		_, _, err = ExecAtWithInput(boot0, tenetPrivilegedCiliumNetworkPolicyYAML, "kubectl", "-n", tenetNecoNamespace, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
	})
}
