package test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var argocdFQDN = testID + "-argocd.gcp0.dev-ne.co"

func prepareArgoCDIngress() {
	argocdFQDN := testID + "-argocd.gcp0.dev-ne.co"
	It("should create HTTPProxy for ArgoCD", func() {
		manifest := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: argocd-server-test
  namespace: argocd
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: argocd-server-cert-test
  routes:
    # For static files and Dex APIs
    - conditions:
        - prefix: /
      services:
        - name: argocd-server-https
          port: 443
      timeoutPolicy:
        response: 2m
        idle: 5m
    # For gRPC APIs
    - conditions:
        - prefix: /
        - header:
            name: content-type
            contains: application/grpc
      services:
        - name: argocd-server
          port: 443
      timeoutPolicy:
        response: 2m
        idle: 5m
`, argocdFQDN)

		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should add argocd service addr entry to /etc/hosts", func() {
		ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
		Expect(err).ShouldNot(HaveOccurred())
		// Save a backup before editing /etc/hosts
		b, err := os.ReadFile("/etc/hosts")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile("./hosts", b, 0644)).NotTo(HaveOccurred())
		f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = f.Write([]byte(ip + " " + argocdFQDN + " \n"))
		Expect(err).ShouldNot(HaveOccurred())
		f.Close()
	})
}

func testArgoCDIngress() {
	It("should confirm Argo CD functionalities", func() {
		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("argocd-server-test", "argocd")
		}).Should(Succeed())

		By("logging in to Argo CD")
		Eventually(func() error {
			output, err := exec.Command(
				"argocd",
				"login",
				argocdFQDN,
				"--insecure",
				"--username",
				"admin",
				"--password",
				loadArgoCDPassword()).Output()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}
			return nil
		}).Should(Succeed())

		for _, path := range []string{"/", "/login", "/applications"} {
			By("requesting to web UI with https path=" + path)
			Eventually(func() error {
				output, err := exec.Command(
					"curl", "-skL", "https://"+argocdFQDN+path,
					"-o", "/dev/null",
					"-H", "Accept: text/html",
					"-w", "%{http_code}\n%{content_type}",
				).Output()
				if err != nil {
					return fmt.Errorf("output: %s, err: %v", output, err)
				}
				fmt.Printf("output: %v\n", string(output)) // DEBUG

				s := strings.Split(string(output), "\n")
				if !strings.Contains(s[0], strconv.Itoa(http.StatusOK)) {
					return fmt.Errorf("http status is not 200: %s", s[0])
				}
				if !strings.Contains(s[1], "text/html; charset=utf-8") {
					return fmt.Errorf("content type is not text/html: %s", s[1])
				}
				return nil
			}).Should(Succeed())
		}

		By("requesting to argocd-dex-server via argocd-server with https")
		Eventually(func() error {
			output, err := exec.Command(
				"curl", "-skL", "https://"+argocdFQDN+"/api/dex/.well-known/openid-configuration",
				"-o", "/dev/null",
				"-w", "%{http_code}\n%{content_type}",
			).Output()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}
			fmt.Printf("output: %v\n", string(output)) // DEBUG

			s := strings.Split(string(output), "\n")
			if !strings.Contains(s[0], strconv.Itoa(http.StatusOK)) {
				return fmt.Errorf("http status is not 200: %s", s[0])
			}
			if !strings.Contains(s[1], "application/json") {
				return fmt.Errorf("content type is not application/json: %s", s[1])
			}
			return nil
		}).Should(Succeed())

		By("requesting to argocd-server with gRPC")
		Eventually(func() error {
			output, err := exec.Command(
				"curl", "-skL", "https://"+argocdFQDN+"/account.AccountService/Read",
				"-H", "Content-Type: application/grpc",
				"-o", "/dev/null",
				"-w", "%{http_code}\n%{content_type}",
			).Output()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}
			fmt.Printf("output: %v\n", string(output)) // DEBUG

			s := strings.Split(string(output), "\n")
			if !strings.Contains(s[0], strconv.Itoa(http.StatusOK)) {
				return fmt.Errorf("http status is not 200: %s", s[0])
			}
			if !strings.Contains(s[1], "application/grpc") {
				return fmt.Errorf("content type is not application/grpc: %s", s[1])
			}
			return nil
		}).Should(Succeed())

		By("requesting to argocd-server with gRPC-Web")
		Eventually(func() error {
			output, err := exec.Command(
				"curl", "-skL", "https://"+argocdFQDN+"/application.ApplicationService/Read",
				"-H", "Content-Type: application/grpc-web+proto",
				"-o", "/dev/null",
				"-w", "%{http_code}\n%{content_type}",
			).Output()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}
			fmt.Printf("output: %v\n", string(output)) // DEBUG

			s := strings.Split(string(output), "\n")
			if !strings.Contains(s[0], strconv.Itoa(http.StatusOK)) {
				return fmt.Errorf("http status is not 200: %s", s[0])
			}
			if !strings.Contains(s[1], "application/grpc-web+proto") {
				return fmt.Errorf("content type is not application/grpc-web+proto: %s", s[1])
			}
			return nil
		}).Should(Succeed())
	})
}
