package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"os/exec"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/metallb.yaml
var metallbYAML string

func prepareMetalLB() {
	It("should deploy load balancer type service", func() {
		By("creating deployments and service")
		tmpl := template.Must(template.New("").Parse(metallbYAML))
		type tmplParams struct {
			TestID string
		}
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, tmplParams{testID})
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "create", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testMetalLB() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDaemonSetNumber("speaker", "metallb-system", -1)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("controller", "metallb-system", 1)
		}).Should(Succeed())

		By("waiting pods are ready")
		Eventually(func() error {
			return checkDeploymentReplicas("testhttpd", "default", 2)
		}).Should(Succeed())
	})

	It("should work", func() {
		By("waiting service are ready")
		var targetIP string
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "service/testhttpd", "-o", "json")
			if err != nil {
				return err
			}

			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return err
			}

			if len(service.Status.LoadBalancer.Ingress) == 0 {
				return errors.New("LoadBalancer status is not updated")
			}

			targetIP = service.Status.LoadBalancer.Ingress[0].IP
			return nil
		}).Should(Succeed())

		By("access service from boot-0 (via cke-localproxy)")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "curl", "http://testhttpd.default.svc/", "-m", "5")
			return err
		}).Should(Succeed())

		By("access service from external")
		Eventually(func() error {
			return exec.Command("ip", "netns", "exec", "external", "curl", targetIP, "-m", "5").Run()
		}).Should(Succeed())

		By("resolve fqdn")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "nslookup", testID+".testhttpd.default.gcp0.dev-ne.co")
			return err
		}).Should(Succeed())

	})
}
