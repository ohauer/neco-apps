package test

import (
	"bytes"
	_ "embed"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/external-dns.yaml
var externalDNSYAML string

func prepareExternalDNS() {
	It("should deploy load balancer type service", func() {
		By("creating deployments and service")
		tmpl := template.Must(template.New("").Parse(externalDNSYAML))
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

func testExternalDNS() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("external-dns", "external-dns", 1)
		}).Should(Succeed())

		By("waiting pods are ready")
		Eventually(func() error {
			return checkDeploymentReplicas("testhttpd", "default", 2)
		}).Should(Succeed())
	})

	It("should work", func() {
		By("resolve fqdn")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "nslookup", testID+".testhttpd.default.gcp0.dev-ne.co")
			return err
		}).Should(Succeed())
	})
}
