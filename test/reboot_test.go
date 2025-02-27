package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// ckeCluster is part of cke.Cluster in github.com/cybozu-go/cke
type ckeCluster struct {
	Nodes []*ckeNode `yaml:"nodes"`
}

// ckeNode is part of cke.Node in github.com/cybozu-go/cke
type ckeNode struct {
	Address      string `yaml:"address"`
	ControlPlane bool   `yaml:"control_plane"`
}

// serfMember is copied from type Member https://godoc.org/github.com/hashicorp/serf/cmd/serf/command#Member
// to prevent much vendoring
type serfMember struct {
	Name   string            `json:"name"`
	Addr   string            `json:"addr"`
	Port   uint16            `json:"port"`
	Tags   map[string]string `json:"tags"`
	Status string            `json:"status"`
	Proto  map[string]uint8  `json:"protocol"`
	// contains filtered or unexported fields
}

// serfMemberContainer is copied from type MemberContainer https://godoc.org/github.com/hashicorp/serf/cmd/serf/command#MemberContainer
// to prevent much vendoring
type serfMemberContainer struct {
	Members []serfMember `json:"members"`
}

func fetchClusterNodes() (map[string]bool, error) {
	stdout, stderr, err := ExecAt(boot0, "ckecli", "cluster", "get")
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}

	cluster := new(ckeCluster)
	err = yaml.Unmarshal(stdout, cluster)
	if err != nil {
		return nil, err
	}

	m := make(map[string]bool)
	for _, n := range cluster.Nodes {
		m[n.Address] = n.ControlPlane
	}
	return m, nil
}

func getSerfMembers() (*serfMemberContainer, error) {
	stdout, stderr, err := ExecAt(boot0, "serf", "members", "-format", "json")
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}
	var result serfMemberContainer
	err = json.Unmarshal(stdout, &result)
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}
	return &result, nil
}

//go:embed testdata/reboot-pod.yaml
var rebootPodYAML []byte

// testRebootAllNodes tests all nodes stop scenario
func testRebootAllNodes() {
	var beforeNodes map[string]bool

	It("fetch cluster nodes", func() {
		var err error
		beforeNodes, err = fetchClusterNodes()
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("stop CKE sabakan integration", func() {
		ExecSafeAt(boot0, "ckecli", "sabakan", "disable")
	})

	It("should stop all CKE service", func() {
		ExecSafeAt(boot0, "sudo", "systemctl", "stop", "cke.service")
		ExecSafeAt(boot1, "sudo", "systemctl", "stop", "cke.service")
		ExecSafeAt(boot2, "sudo", "systemctl", "stop", "cke.service")
	})

	It("should stop all kube-controller-manager and delete all Pod", func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "-l", "node-role.kubernetes.io/control-plane=true", "-ojsonpath='{.items..metadata.name}'")
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		cpAddrs := strings.Split(string(stdout), " ")
		for _, a := range cpAddrs {
			ExecSafeAt(boot0, "ckecli", "ssh", a, "docker", "stop", "kube-controller-manager")
		}
		ExecSafeAt(boot0, "kubectl", "delete", "pod", "-A", "--all", "--force")
	})

	It("reboots all nodes", func() {
		By("getting machines list")
		stdout, _, err := ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).ShouldNot(HaveOccurred())
		var machines []sabakan.Machine
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())

		By("shutting down all nodes")
		for _, m := range machines {
			if m.Spec.Role == "boot" {
				continue
			}
			stdout, stderr, err := ExecAt(boot0, "neco", "power", "stop", m.Spec.IPv4[0])
			Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}

		By("waiting for start of rebooting")
		preReboot := make(map[string]bool)
		for _, m := range machines {
			if m.Spec.Role == "boot" {
				continue
			}
			preReboot[m.Spec.IPv4[0]] = true
		}
		Eventually(func() error {
			result, err := getSerfMembers()
			if err != nil {
				return err
			}
			for _, member := range result.Members {
				addrs := strings.Split(member.Addr, ":")
				if len(addrs) != 2 {
					return fmt.Errorf("unexpected addr: %s", member.Addr)
				}
				addr := addrs[0]
				if preReboot[addr] && member.Status != "alive" {
					delete(preReboot, addr)
				}
			}
			if len(preReboot) > 0 {
				return fmt.Errorf("some nodes are still starting reboot: %v", preReboot)
			}
			return nil
		}).Should(Succeed())

		By("starting all nodes")
		for _, m := range machines {
			if m.Spec.Role == "boot" {
				continue
			}
			stdout, stderr, err := ExecAt(boot0, "neco", "power", "start", m.Spec.IPv4[0])
			Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}

		By("waiting for recovery of all nodes")
		Eventually(func() error {
			nodes, err := fetchClusterNodes()
			if err != nil {
				return err
			}
			result, err := getSerfMembers()
			if err != nil {
				return err
			}
		OUTER:
			for k := range nodes {
				for _, m := range result.Members {
					addrs := strings.Split(m.Addr, ":")
					if len(addrs) != 2 {
						return fmt.Errorf("unexpected addr: %s", m.Addr)
					}
					addr := addrs[0]
					if addr == k {
						if m.Status != "alive" {
							return fmt.Errorf("still not alive: %s, %v", k, m)
						}
						continue OUTER
					}
				}
				return fmt.Errorf("cannot find in serf members: %s", k)
			}
			return nil
		}).Should(Succeed())
	})

	It("fetch cluster nodes", func() {
		Eventually(func() error {
			afterNodes, err := fetchClusterNodes()
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(beforeNodes, afterNodes) {
				return fmt.Errorf("cluster nodes mismatch after reboot: before=%v after=%v", beforeNodes, afterNodes)
			}

			return nil
		}).Should(Succeed())
	})

	It("sets all nodes' machine state to healthy", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "sabactl", "machines", "get")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var machines []sabakan.Machine
			err = json.Unmarshal(stdout, &machines)
			if err != nil {
				return err
			}

			for _, m := range machines {
				if m.Spec.Role == "boot" {
					continue
				}
				stdout := ExecSafeAt(boot0, "sabactl", "machines", "get-state", m.Spec.Serial)
				state := string(bytes.TrimSpace(stdout))
				if state != "healthy" {
					return fmt.Errorf("sabakan machine state of %s is not healthy: %s", m.Spec.Serial, state)
				}
			}

			return nil
		}).Should(Succeed())
	})

	It("should start all CKE service", func() {
		ExecSafeAt(boot0, "sudo", "systemctl", "start", "cke.service")
		ExecSafeAt(boot1, "sudo", "systemctl", "start", "cke.service")
		ExecSafeAt(boot2, "sudo", "systemctl", "start", "cke.service")
	})

	It("re-enable CKE sabakan integration", func() {
		ExecSafeAt(boot0, "ckecli", "sabakan", "enable")
	})

	It("wait for Kubernetes cluster to become ready", func() {
		By("waiting nodes")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var nl corev1.NodeList
			err = json.Unmarshal(stdout, &nl)
			if err != nil {
				return err
			}

			if len(nl.Items) != numNodes {
				return fmt.Errorf("too few nodes: %d", len(nl.Items))
			}
		OUTER:
			for _, n := range nl.Items {
				for _, cond := range n.Status.Conditions {
					if cond.Type != corev1.NodeReady {
						continue
					}
					if cond.Status != corev1.ConditionTrue {
						return fmt.Errorf("node %s is not ready", n.Name)
					}
					continue OUTER
				}
				return fmt.Errorf("node %s has no readiness status", n.Name)
			}
			return nil
		}).Should(Succeed())

		By("confirming that pods can be deployed")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, rebootPodYAML, "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("waits for Kubernetes resources to become ready", func() {
		By("cofirming that deployment is ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "deployment", "-A", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var list appsv1.DeploymentList
			err = json.Unmarshal(stdout, &list)
			if err != nil {
				return fmt.Errorf("err: %v, stdout: %s", err, stdout)
			}

			for _, d := range list.Items {
				// default value of replicas is 1
				replicas := int32(1)
				if d.Spec.Replicas != nil {
					replicas = *d.Spec.Replicas
				}

				if replicas != d.Status.AvailableReplicas {
					return fmt.Errorf(
						"the number of replicas of Deployment %s/%s should be %d: %d",
						d.Namespace, d.Name, replicas, d.Status.AvailableReplicas,
					)
				}
			}
			return nil
		}).Should(Succeed())

		By("cofirming that statefulset is ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "statefulset", "-A", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var list appsv1.StatefulSetList
			err = json.Unmarshal(stdout, &list)
			if err != nil {
				return fmt.Errorf("err: %v, stdout: %s", err, stdout)
			}

			for _, d := range list.Items {
				// default value of replicas is 1
				replicas := int32(1)
				if d.Spec.Replicas != nil {
					replicas = *d.Spec.Replicas
				}

				if replicas != d.Status.AvailableReplicas {
					return fmt.Errorf(
						"the number of replicas of StatefulSet %s/%s should be %d: %d",
						d.Namespace, d.Name, replicas, d.Status.AvailableReplicas,
					)
				}
			}
			return nil
		}).Should(Succeed())

		By("cofirming that daemonset is ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "daemonset", "-A", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var list appsv1.DaemonSetList
			err = json.Unmarshal(stdout, &list)
			if err != nil {
				return fmt.Errorf("err: %v, stdout: %s", err, stdout)
			}

			for _, d := range list.Items {
				if d.Status.DesiredNumberScheduled <= 0 {
					return fmt.Errorf("%s daemonset's desiredNumberScheduled is not updated", d.Name)
				}

				if d.Status.DesiredNumberScheduled != d.Status.NumberAvailable {
					return fmt.Errorf("not all nodes running %s daemonset: %d", d.Name, d.Status.NumberAvailable)
				}
			}
			return nil
		}).Should(Succeed())
	})
}
