package test

import (
	_ "embed"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed testdata/accurate-subnamespace.yaml
var accurateSubNamespaceYAML []byte

//go:embed testdata/accurate-invalid-subnamespace.yaml
var accurateInvalidSubNamespaceYAML []byte

const accurateParentNamespaceName = "dev-accurate-parent"
const accurateChildNamespaceName = "dev-accurate-child"

// accuratePropagatedNamespaceLabels is labels to be propagated.
// `team` is not included because it requires special handling.
var accuratePropagatedNamespaceLabels = []string{
	"development",
	"pod-security.cybozu.com/policy",
}

//go:embed testdata/accurate-propagated.yaml
var accuratePropagatedYAML []byte

var accuratePropagatedResourceKinds = []string{
	"Role",
	"RoleBinding",
	"Secret",
}

func prepareAccurate() {
	It("should create namespaces for accurate", func() {
		createNamespaceIfNotExists(accurateParentNamespaceName, false)
		ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, "accurate.cybozu.com/type=root")
		// `team` should have actual team name.
		ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, "team=neco")
		for _, k := range accuratePropagatedNamespaceLabels {
			ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, k+"=to-be-propagated")
		}
	})
}

func testAccurate() {
	It("should check Accurate", func() {
		By("creating child namespace by creating SubNamespace")
		stdout, stderr, err := ExecAtWithInput(boot0, accurateSubNamespaceYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "ns", accurateChildNamespaceName, "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to create child namespace: %s: %w", string(stderr), err)
			}
			var meta struct {
				metav1.TypeMeta   `json:",inline"`
				metav1.ObjectMeta `json:"metadata,omitempty"`
			}
			err = json.Unmarshal(stdout, &meta)
			if err != nil {
				return err
			}
			if meta.ObjectMeta.Labels == nil {
				return fmt.Errorf("namespace labels are not propagated")
			}
			if meta.ObjectMeta.Labels["team"] != "neco" {
				return fmt.Errorf("namespace label 'team' is not propagated")
			}
			for _, k := range accuratePropagatedNamespaceLabels {
				if meta.ObjectMeta.Labels[k] != "to-be-propagated" {
					return fmt.Errorf("namespace label '%s' is not propagated", k)
				}
			}
			if meta.ObjectMeta.Labels["cybozu.com/alert-group"] != "sample" {
				return fmt.Errorf("namespace label 'cybozu.com/alert-group' is not set")
			}
			if _, ok := meta.ObjectMeta.Labels["cybozu.com/not-allowed"]; ok {
				return fmt.Errorf("disallowed label is set")
			}
			return nil
		}).Should(Succeed())

		By("creating invalid child namespace by creating SubNamespace")
		_, stderr, err = ExecAtWithInput(boot0, accurateInvalidSubNamespaceYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "vsubnamespace.kb.io" denied the request`))

		By("checking whether deletion of parent namespace is blocked by webhook")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "ns", accurateParentNamespaceName)
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		By("checking certain types of resources are propagated")
		stdout, stderr, err = ExecAtWithInput(boot0, accuratePropagatedYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		Eventually(func() error {
			for _, kind := range accuratePropagatedResourceKinds {
				_, stderr, err := ExecAt(boot0, "kubectl", "get", kind, "-n", accurateChildNamespaceName, "propagated")
				if err != nil {
					return fmt.Errorf("failed to find propagated resource %s: %s: %w", kind, string(stderr), err)
				}
			}
			return nil
		}).Should(Succeed())

		By("deleting child namespace by deleting SubNamespace")
		ExecSafeAt(boot0, "kubectl", "annotate", "SubNamespace", "-n", accurateParentNamespaceName, accurateChildNamespaceName, "admission.cybozu.com/i-am-sure-to-delete="+accurateChildNamespaceName)
		ExecSafeAt(boot0, "kubectl", "delete", "SubNamespace", "-n", accurateParentNamespaceName, accurateChildNamespaceName)

		Eventually(func() error {
			_, _, err := ExecAt(boot0, "kubectl", "get", "ns", accurateChildNamespaceName)
			if err == nil {
				return fmt.Errorf("failed to delete child namespace")
			}
			return nil
		}).Should(Succeed())
	})
}
