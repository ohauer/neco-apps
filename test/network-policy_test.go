package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/cybozu-go/log"
	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/network-policy.yaml
var networkPolicyYAML []byte

func prepareNetworkPolicy() {
	It("should prepare test pods in test-netpol namespace", func() {
		By("preparing namespace")
		createNamespaceIfNotExists("test-netpol", false)

		By("deploying pods")
		_, stderr, err := ExecAtWithInput(boot0, networkPolicyYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should wait for patched pods to become ready", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("squid", "internet-egress", 2)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("squid", "customer-egress", 2)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("unbound", "internet-egress", 2)
		}).Should(Succeed())

		Eventually(func() error {
			return checkDeploymentReplicas("vmagent-vmagent-smallset", "monitoring", 1)
		}).Should(Succeed())

		const vmagentLargesetCount = 3
		Eventually(func() error {
			return checkDeploymentReplicas("vmagent-vmagent-largeset", "monitoring", vmagentLargesetCount)
		}).Should(Succeed())
	})

	It("should config cilium PolicyAuditMode=false", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "kube-system", "get", "pods", "-l", "k8s-app=cilium", "-o", "json")
			if err != nil {
				return fmt.Errorf("stderr: %s: %w", string(stderr), err)
			}
			podList := &corev1.PodList{}
			if err := json.Unmarshal(stdout, podList); err != nil {
				return err
			}

			for _, item := range podList.Items {
				stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "kube-system", "exec", item.Name, "--", "cilium", "config", "PolicyAuditMode=false")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})
}

func testNetworkPolicy() {
	It("should pass/block packets appropriately", func() {
		By("waiting for testhttpd pods")
		Eventually(func() error {
			return checkDeploymentReplicas("testhttpd", "test-netpol", 2)
		}).Should(Succeed())

		By("waiting for ubuntu pod")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "default", "exec", "ubuntu", "--", "date")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		testhttpdPodList := new(corev1.PodList)
		nodeList := new(corev1.NodeList)
		var nodeIP string
		var apiServerIP string
		var apiServerPort string

		By("getting httpd pod list")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "test-netpol", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		err = json.Unmarshal(stdout, testhttpdPodList)
		Expect(err).NotTo(HaveOccurred())

		By("getting all node list")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "node", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		err = json.Unmarshal(stdout, nodeList)
		Expect(err).NotTo(HaveOccurred())

		By("getting a certain node IP address")
	OUTER:
		for _, node := range nodeList.Items {
			for _, addr := range node.Status.Addresses {
				if addr.Type == "InternalIP" {
					nodeIP = addr.Address
					break OUTER
				}
			}
		}
		Expect(nodeIP).NotTo(BeEmpty())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "config", "view", "--output=jsonpath={.clusters[0].cluster.server}")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		u, err := url.Parse(string(stdout))
		Expect(err).NotTo(HaveOccurred(), "server: %s", stdout)
		splitHost := strings.Split(u.Host, ":")
		apiServerIP = splitHost[0]
		Expect(apiServerIP).NotTo(BeEmpty(), "server: %s", stdout)
		if len(splitHost) >= 2 {
			apiServerPort = splitHost[1]
		} else {
			Expect(u.Scheme).To(Equal("https"), "server: %s", stdout)
			apiServerPort = "443"
		}

		By("resolving hostname inside cluster by cluster-dns")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "nslookup", "-timeout=10", "testhttpd.test-netpol")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("resolving hostname outside cluster by unbound")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "nslookup", "-timeout=10", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("checking if it filters packets from squid/unbound of internet-egress to private network")
		includeUnbound := true
		testFiltersForInternetEgress("internet-egress", testhttpdPodList.Items[0].Status.PodIP, nodeIP, includeUnbound)

		By("checking if it filters packets from squid/unbound of customer-egress to private network")
		includeUnbound = false
		testFiltersForInternetEgress("customer-egress", testhttpdPodList.Items[0].Status.PodIP, nodeIP, includeUnbound)

		By("checking if it passes packets to node network for system services")
		By("accessing DNS port of some node")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing API server port of control plane node")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", apiServerIP, apiServerPort, "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("getting vmagent-smallset pod name")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "-n=monitoring", "-l=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-smallset", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		podName := string(stdout)

		By("adding an ubuntu-debug container as an ephemeral container to vmagent-smallset")
		stdout, stderr, err = ExecAt(boot0,
			"kubectl", "debug", podName,
			"-n=monitoring",
			"--container=ubuntu",
			"--image=quay.io/cybozu/ubuntu-debug:20.04",
			"--target=vmagent",
			"--", "pause",
		)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing node-expoter port of some node as vmagent-smallset")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "monitoring", "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "9100", "-e", "X")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("getting vmagent-largeset pod name")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "-n=monitoring", "-l=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-largeset", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		podName = string(stdout)

		By("adding an ubuntu-debug container as an ephemeral container to vmagent-largeset")
		stdout, stderr, err = ExecAt(boot0,
			"kubectl", "debug", podName,
			"-n=monitoring",
			"--container=ubuntu",
			"--image=quay.io/cybozu/ubuntu-debug:20.04",
			"--target=vmagent",
			"--", "pause",
		)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing node-expoter port of some node as vmagent-largeset")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "monitoring", "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "9100", "-e", "X")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("checking if it filters icmp packets to BMC/Node/Bastion/switch networks")
		stdout, stderr, err = ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		var machines []sabakan.Machine
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())

		ping := func(addr string) error {
			_, _, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "ping", "-c", "1", "-W", "3", addr)
			if err != nil {
				return err
			}
			log.Error("ping should be failed, but it was succeeded", map[string]interface{}{
				"target": addr,
			})
			return nil
		}
		for _, m := range machines {
			bmcAddr := m.Spec.BMC.IPv4
			node0Addr := m.Spec.IPv4[0]
			Expect(ping(bmcAddr)).To(HaveOccurred())
			Expect(ping(node0Addr)).To(HaveOccurred())
		}
		// Bastion
		Expect(ping(boot0)).To(HaveOccurred())
		// Switch
		Expect(ping("10.72.2.0")).To(HaveOccurred())
	})
}

func testFiltersForInternetEgress(namespace string, localPodIP, nodeIP string, includeUnbound bool) {
	By("adding an ubuntu-debug container as an ephemeral container to squid")
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n="+namespace, "-l=app.kubernetes.io/name=squid", "-o", "json")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

	squidPodList := new(corev1.PodList)
	err = json.Unmarshal(stdout, squidPodList)
	Expect(err).NotTo(HaveOccurred())

	for _, pod := range squidPodList.Items {
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "debug", pod.Name,
			"-n="+namespace,
			"--container=ubuntu",
			"--image=quay.io/cybozu/ubuntu-debug:20.04",
			"--target=squid",
			"--", "pause",
		)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	}

	By("accessing to local IP")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "-n="+namespace, "get", "pods", "-o=json")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	podList := new(corev1.PodList)
	err = json.Unmarshal(stdout, podList)
	Expect(err).NotTo(HaveOccurred())

	for _, pod := range podList.Items {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", pod.Namespace, pod.Name, "--", "curl", localPodIP, "-m", "5")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	}

	By("accessing DNS port of some node as squid")
	Eventually(func() error {
		stdout, _, err = ExecAt(boot0, "kubectl", "get", "pods", "-n="+namespace, "-l=app.kubernetes.io/name=squid", "-o", "json")
		if err != nil {
			return err
		}

		squidPodList := new(corev1.PodList)
		err = json.Unmarshal(stdout, squidPodList)
		if err != nil {
			return err
		}

		var podName string
	OUTER:
		for _, pod := range squidPodList.Items {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady {
					podName = pod.Name
					break OUTER
				}
			}
		}
		if podName == "" {
			return errors.New("podName should not be blank")
		}

		stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n="+namespace, "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		var sshError *ssh.ExitError
		var execError *exec.ExitError
		switch {
		case errors.As(err, &sshError):
			if sshError.ExitStatus() != 124 {
				return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", sshError.ExitStatus(), stdout, stderr, err)
			}
		case errors.As(err, &execError):
			if execError.ExitCode() != 124 {
				return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", execError.ExitCode(), stdout, stderr, err)
			}
		default:
			return fmt.Errorf("telnet should fail with timeout; stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())

	if !includeUnbound {
		return
	}

	By("adding an ubuntu-debug container as an ephemeral container to unbound")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "-n="+namespace, "-l=app.kubernetes.io/name=unbound", "-o", "json")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

	unboundPodList := new(corev1.PodList)
	err = json.Unmarshal(stdout, unboundPodList)
	Expect(err).NotTo(HaveOccurred())

	for _, pod := range unboundPodList.Items {
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "debug", pod.Name,
			"-n="+namespace,
			"--container=ubuntu",
			"--image=quay.io/cybozu/ubuntu-debug:20.04",
			"--target=unbound",
			"--", "pause",
		)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	}

	By("getting unbound pod name")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "-n="+namespace, "-l=app.kubernetes.io/name=unbound", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	unboundPodName := string(stdout)

	By("accessing DNS port of some node as unbound")
	Eventually(func() error {
		stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n="+namespace, "exec", "-i", unboundPodName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		var sshError *ssh.ExitError
		var execError *exec.ExitError
		switch {
		case errors.As(err, &sshError):
			if sshError.ExitStatus() != 124 {
				return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", sshError.ExitStatus(), stdout, stderr, err)
			}
		case errors.As(err, &execError):
			if execError.ExitCode() != 124 {
				return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", execError.ExitCode(), stdout, stderr, err)
			}
		default:
			return fmt.Errorf("telnet should fail with timeout; stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}
