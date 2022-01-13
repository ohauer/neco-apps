package test

import (
	"bytes"
	"encoding/json"
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

		By("loading meows-secret.json")
		data, err := os.ReadFile(meowsSecretFile)
		Expect(err).NotTo(HaveOccurred())
		env := make(map[string]string)
		err = json.Unmarshal(data, &env)
		Expect(err).NotTo(HaveOccurred())

		By("creating temporally file for secrets")
		fileCreateSafeAt(boot0, "github_app_id", env["github_app_id"])
		fileCreateSafeAt(boot0, "github_app_installation_id", env["github_app_installation_id"])
		fileCreateSafeAt(boot0, "github_app_private_key", env["github_app_private_key"])

		By("creating secrets in the " + meowsRunnerNS)
		_ = ExecSafeAt(boot0, "kubectl", "create", "secret", "generic", "meows-github-cred",
			"-n", meowsRunnerNS,
			"--from-file=app-id=github_app_id",
			"--from-file=app-installation-id=github_app_installation_id",
			"--from-file=app-private-key=github_app_private_key",
		)

		By("creating secrets in a tenant namespace (" + tenantDevNS + ")")
		_ = ExecSafeAt(boot0, "kubectl", "create", "secret", "generic", "meows-github-cred",
			"-n", tenantDevNS,
			"--from-file=app-id=github_app_id",
			"--from-file=app-installation-id=github_app_installation_id",
			"--from-file=app-private-key=github_app_private_key",
		)

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
	It("should deploy meows-controller and slack-agent", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}

		Eventually(func() error {
			return checkDeploymentReplicas("meows-controller", "meows", 2)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("slack-agent", "meows", 2)
		}).Should(Succeed())

		By("accessing to slack-agent: should be allow")
		runnerPoolName := genRunnerPoolName()
		// This command does not want to check the communication with the slack api, so ignore the error.
		// Cannot be checked by curl command or other methods.
		ExecAt(boot0, "kubectl", "exec", "-n", "meows", "deploy/meows-controller", "--", "/usr/local/bin/meows", "slackagent", "send", runnerPoolName, "success", "-s", "http://slack-agent.meows.svc", "-c", "test1")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "logs", "-n", "meows", "-l", "app.kubernetes.io/component=slack-agent", "|", "grep", "-e", "'success to send slack message'", "-e", "'failed to send slack message'", "|", "grep", "-q", runnerPoolName)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).ShouldNot(HaveOccurred())
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
