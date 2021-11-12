package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

const (
	meowsRunnerNS   = "meows-runner"
	meowsSecretFile = "meows-secret.json"
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

		hostname, err := os.Hostname()
		Expect(err).NotTo(HaveOccurred())

		var buf bytes.Buffer
		tpl := template.Must(template.ParseFiles(filepath.Join(".", "testdata", "meows-runnerpool.tmpl.yaml")))
		tpl.Execute(&buf, map[string]string{
			"RunnerPoolName": "runnerpool-" + hostname,
		})

		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "--namespace", meowsRunnerNS, "-f", "-")
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

		hostname, err := os.Hostname()
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			return checkDeploymentReplicas("runnerpool-"+hostname, "meows-runner", 1)
		}).Should(Succeed())

		By("checking that runner pods become online")
		Eventually(func() error {
			result, err := queryMetrics(MonitoringLargeset, `count(meows_runner_online)`)
			if err != nil {
				return err
			}
			if len(result.Data.Result) <= 0 {
				return fmt.Errorf("no count metrics is retrieved")
			}
			onlineRunners := int(result.Data.Result[0].Value)
			if onlineRunners != 1 {
				return fmt.Errorf("there is not an online runner pod, the metrics of count(meows_runner_online) is %d", onlineRunners)
			}
			return nil
		}).Should(Succeed())

		By("getting runner pod list")
		runnerPodList := new(corev1.PodList)
		stdout := ExecSafeAt(boot0, "kubectl", "get", "pods", "-n", meowsRunnerNS, "-l=app.kubernetes.io/name=meows,app.kubernetes.io/component=runner", "-o=json")
		err = json.Unmarshal(stdout, runnerPodList)
		Expect(err).ShouldNot(HaveOccurred())

		By("accessing to private IP: should be deny")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", runnerPodList.Items[0].Namespace, runnerPodList.Items[0].Name, "--", "curl", "testhttpd.test-netpol", "-m", "5")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing to slack-agent: should be allow")
		ExecSafeAt(boot0, "kubectl", "exec", "-n", runnerPodList.Items[0].Namespace, runnerPodList.Items[0].Name, "--", "/usr/local/bin/meows", "slackagent", "send", runnerPodList.Items[0].Name, "success", "-s", "http://slack-agent.meows.svc", "-c", "test1", "-n", runnerPodList.Items[0].Namespace)
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "logs", "-n", "meows", "-l", "app.kubernetes.io/component=slack-agent", "|", "grep", "'success to send slack message'", "|", "grep", "-q", runnerPodList.Items[0].Name)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).ShouldNot(HaveOccurred())
	})
}
