package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	meowsRunnerNS   = "meows-runner"
	meowsSecretFile = "meows-secret.json"
	tenantDevNS     = "dev-maneki"
	tenantTeam      = "maneki"
)

func meowsDisabled() bool {
	_, err := os.Stat(meowsSecretFile)
	return err != nil
}

func prepareMeows() {
	It("should create runner pool", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}
		runnerPoolName := genRunnerPoolName()

		By("creating a RunnerPool in the " + meowsRunnerNS)
		var buf bytes.Buffer
		tpl := template.Must(template.ParseFiles(filepath.Join(".", "testdata", "meows-runnerpool.tmpl.yaml")))
		tpl.Execute(&buf, map[string]string{
			"RunnerPoolName": runnerPoolName,
			"Namespace":      meowsRunnerNS,
		})
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("creating a RunnerPool in a tenant namespace (" + tenantDevNS + ") as a tenant team member")
		tpl = template.Must(template.ParseFiles(filepath.Join(".", "testdata", "meows-runnerpool.tmpl.yaml")))
		tpl.Execute(&buf, map[string]string{
			"RunnerPoolName": runnerPoolName,
			"Namespace":      tenantDevNS,
		})
		_, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-", "--as=test", "--as-group="+tenantTeam, "--as-group=system:authenticated")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testMeows() {
	It("should deploy meows-controller", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}

		Eventually(func() error {
			return checkDeploymentReplicas("meows-controller", "meows", 2)
		}).Should(Succeed())
	})

	It("should deploy slack-agent", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}

		Eventually(func() error {
			return checkDeploymentReplicas("slack-agent", "meows", 2)
		}).Should(Succeed())
	})

	It("should deploy runner pool", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}
		runnerPoolName := genRunnerPoolName()
		checkRunnerPool(runnerPoolName, meowsRunnerNS)
		checkRunnerPool(runnerPoolName, tenantDevNS)

		By("checking network policy for runner pods in the " + meowsRunnerNS)
		connectionShouldBeDenied(meowsRunnerNS, "deploy/"+runnerPoolName, "http://argocd-server.argocd.svc")
		connectionShouldBeDenied(meowsRunnerNS, "deploy/"+runnerPoolName, "http://slack-agent.meows.svc")
		connectionShouldBeDenied(meowsRunnerNS, "deploy/"+runnerPoolName, "https://kubernetes.default.svc")

		By("checking network policy for runner pods in a tenant namespace (" + tenantDevNS + ")")
		connectionShouldBeDenied(tenantDevNS, "deploy/"+runnerPoolName, "http://argocd-server.argocd.svc")
		connectionShouldBeDenied(tenantDevNS, "deploy/"+runnerPoolName, "http://slack-agent.meows.svc")
		connectionShouldBeAllowed(tenantDevNS, "deploy/"+runnerPoolName, "https://kubernetes.default.svc")
	})
}

func genRunnerPoolName() string {
	hostname, err := os.Hostname()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return "runnerpool-" + hostname
}

func checkRunnerPool(name, namespace string) {
	EventuallyWithOffset(1, func() error {
		return checkDeploymentReplicas(name, namespace, 1)
	}).Should(Succeed())

	By("checking that runner pods become online")
	query := `count(meows_runner_online{runnerpool="` + namespace + "/" + name + `"})`
	EventuallyWithOffset(1, func() error {
		result, err := queryMetrics(MonitoringLargeset, query)
		if err != nil {
			return err
		}
		if len(result.Data.Result) <= 0 {
			return fmt.Errorf("no count metrics is retrieved")
		}
		onlineRunners := int(result.Data.Result[0].Value)
		if onlineRunners != 1 {
			return fmt.Errorf("there is not an online runner pod, the metrics of %s is %d", query, onlineRunners)
		}
		return nil
	}).Should(Succeed())
}

func connectionShouldBeAllowed(namespace, from, to string) {
	EventuallyWithOffset(1, func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", namespace, from, "--", "curl", "-k", "-sS", "-m5", to)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}

func connectionShouldBeDenied(namespace, from, to string) {
	// When the connection is denied by NetworkPolicy, curl will time out with the following error message.
	// > curl: (28) Connection timed out after 5000 milliseconds
	// > command terminated with exit code 28
	EventuallyWithOffset(1, func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", namespace, from, "--", "curl", "-k", "-sS", "-m5", to)
		if err == nil {
			return fmt.Errorf("connection is allowed, stdout: %s, stderr: %s", stdout, stderr)
		}
		if !strings.Contains(string(stderr), "curl: (28) Connection timed out") {
			return fmt.Errorf("curl command is not timed out; stdout: %s, stderr: %s", stdout, stderr)
		}
		return nil
	}).Should(Succeed())
}
