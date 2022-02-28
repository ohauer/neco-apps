package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	//go:embed testdata/cattage-tenant.yaml
	tenantYAML []byte
	//go:embed testdata/cattage-subnamespace.yaml
	subNamespaceYAML []byte
	//go:embed testdata/cattage-application.yaml
	applicationYAML []byte
)

func prepareCattage() {
	It("should create tenant and application", func() {
		By("creating tenant")
		_, stderr, err := ExecAtWithInput(boot0, tenantYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "kubectl", "get", "ns", "app-my-team")
			if err != nil {
				return fmt.Errorf("failed to create root namespace: %s: %w", string(stderr), err)
			}
			return nil
		}).Should(Succeed())

		By("creating sub-namespace")
		_, stderr, err = ExecAtWithInput(boot0, subNamespaceYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "kubectl", "get", "ns", "my-team-child")
			if err != nil {
				return fmt.Errorf("failed to create sub namespace: %s: %w", string(stderr), err)
			}
			return nil
		}).Should(Succeed())

		By("creating application")
		_, stderr, err = ExecAtWithInput(boot0, applicationYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func application() *unstructured.Unstructured {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})
	return app
}
func testCattage() {
	It("should sync application", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "app", "-n", "my-team-child", "sample", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get application: %s: %w", string(stderr), err)
			}
			app := application()
			if err := json.Unmarshal(stdout, app); err != nil {
				return err
			}
			healthStatus, found, err := unstructured.NestedString(app.UnstructuredContent(), "status", "health", "status")
			if err != nil {
				return err
			}
			if !found {
				return errors.New("status not found")
			}
			if healthStatus != "Healthy" {
				return errors.New("status is not healthy")
			}

			syncStatus, found, err := unstructured.NestedString(app.UnstructuredContent(), "status", "sync", "status")
			if err != nil {
				return err
			}
			if !found {
				return errors.New("status not found")
			}
			if syncStatus != "Synced" {
				return errors.New("status is not synced")
			}

			return nil
		}).Should(Succeed())
	})
}
