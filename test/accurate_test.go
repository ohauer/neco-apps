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

const accurateParentNamespaceName = "accurate-parent"
const accurateChildNamespaceName = "accurate-child"

var accuratePropagatedNamespaceLabels = []string{
	"team",
	"development",
}
var accuratePropagatedNamespaceAnnotations = []string{
	"metallb.universe.tf/address-pool",
}

//go:embed testdata/accurate-propagated.yaml
var accuratePropagatedYAML []byte

var accuratePropagatedResourceKinds = []string{
	"Role",
	"RoleBinding",
	"Secret",
}

func prepareAccurate() {
	It("should create namepaces for accurate", func() {
		createNamespaceIfNotExists(accurateParentNamespaceName, false)
		ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, "accurate.cybozu.com/type=root")
	})
}

func testAccurate() {
	It("should check Accurate", func() {
		By("adding labels and annotations to be propagated")
		for _, k := range accuratePropagatedNamespaceLabels {
			ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, k+"=to-be-propagated")
		}
		for _, k := range accuratePropagatedNamespaceAnnotations {
			ExecSafeAt(boot0, "kubectl", "annotate", "namespace", accurateParentNamespaceName, k+"=to-be-propagated")
		}

		By("creating child namespace by SubNamespace")
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
			for _, k := range accuratePropagatedNamespaceLabels {
				if meta.ObjectMeta.Labels[k] != "to-be-propagated" {
					return fmt.Errorf("namespace label '%s' is not propagated", k)
				}
			}
			if meta.ObjectMeta.Annotations == nil {
				return fmt.Errorf("namespace annotations are not propagated")
			}
			for _, k := range accuratePropagatedNamespaceAnnotations {
				if meta.ObjectMeta.Annotations[k] != "to-be-propagated" {
					return fmt.Errorf("namespace annotation '%s' is not propagated", k)
				}
			}
			return nil
		}).Should(Succeed())

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

		By("deleting team label from test namespace")
		// team-management_test.go will fail if the label exists.
		ExecSafeAt(boot0, "kubectl", "label", "namespace", accurateParentNamespaceName, "team-")
	})
}
