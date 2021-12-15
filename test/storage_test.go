package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var nss = []string{"ceph-hdd", "ceph-ssd", "ceph-object-store"}

//go:embed testdata/storage-load.yaml
var storageLoadYAML []byte

func prepareLoadPods() {
	It("should deploy pods", func() {
		stdout, stderr, err := ExecAtWithInput(boot0, storageLoadYAML, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		Eventually(func() error {
			return checkDeploymentReplicas("addload-for-ss", "default", 2)
		}).Should(Succeed())
	})
}

//go:embed testdata/storage-obc.yaml
var storageOBCYAML []byte

//go:embed testdata/storage-rbd.yaml
var storageRBDYAML string

func prepareRookCeph() {
	It("should apply a OBC resource and a POD for testRookRGW", func() {
		_, stderr, err := ExecAtWithInput(boot0, storageOBCYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should create a POD for testRookRBD", func() {
		tmpl := template.Must(template.New("").Parse(storageRBDYAML))
		for _, storageClassName := range []string{"ceph-ssd-block"} {
			buf := new(bytes.Buffer)
			err := tmpl.Execute(buf, storageClassName)
			Expect(err).NotTo(HaveOccurred())

			_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		}
	})
}

func testRookOperator() {
	for _, ns := range nss {
		By("checking rook-ceph-operator Deployment for "+ns, func() {
			Eventually(func() error {
				return checkDeploymentReplicas("rook-ceph-operator", ns, 1)
			}).Should(Succeed())
		})

		By("checking ceph-tools Deployment for "+ns, func() {
			Eventually(func() error {
				err := checkDeploymentReplicas("rook-ceph-tools", ns, 1)
				if err != nil {
					return err
				}

				stdout, _, err := ExecAt(boot0, "kubectl", "get", "pod", "--namespace="+ns, "-l", "app=rook-ceph-tools", "-o=json")
				if err != nil {
					return err
				}

				pods := new(corev1.PodList)
				err = json.Unmarshal(stdout, pods)
				if err != nil {
					return err
				}

				podName := pods.Items[0].Name
				_, _, err = ExecAt(boot0, "kubectl", "exec", "--namespace="+ns, podName, "--", "ceph", "status")
				if err != nil {
					return err
				}
				return nil
			}).Should(Succeed())
		})
	}
}

func testClusterStable() {
	for _, ns := range nss {
		By("checking stability of rook/ceph cluster "+ns, func() {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "deployment/rook-ceph-operator", "-o=json")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			deploy := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deploy)
			Expect(err).ShouldNot(HaveOccurred(), "json=%s", stdout)

			imageString := deploy.Spec.Template.Spec.Containers[0].Image
			re := regexp.MustCompile(`:(.+)\.[\d]+$`)
			group := re.FindSubmatch([]byte(imageString))
			expectRookVersion := "v" + string(group[1])

			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "cephcluster", ns, "-o", "jsonpath='{.spec.mon.count}'")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			numMonExpected, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)

			numOsdExpected := getNumOsd(ns)

			numRgwExpected := 0
			if ns == "ceph-object-store" {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "cephobjectstore", "-o", "jsonpath='{.items[*].spec.gateway.instances}'")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
				nums := strings.Fields(string(stdout))
				for _, num := range nums {
					n, err := strconv.Atoi(num)
					Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
					numRgwExpected += n
				}
			}

			By("checking deployments versions are equal to the requiring")
			Eventually(func() error {
				// Confirm deployment version and pod available counts.
				stdout, _, err = ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment", "-o=json")
				if err != nil {
					return err
				}

				deployments := new(appsv1.DeploymentList)
				err = json.Unmarshal(stdout, deployments)
				if err != nil {
					return err
				}

				st := time.Now()
				for {
					if time.Since(st) > 30*time.Second {
						break
					}
					err := confirmCephPod(ns)
					if err != nil {
						return err
					}
					time.Sleep(1 * time.Second)
				}

				var numMon, numOsd, numRgw int
				for _, deployment := range deployments.Items {
					switch deployment.Labels["app"] {
					case "rook-ceph-mon":
						numMon++
					case "rook-ceph-osd":
						numOsd++
					case "rook-ceph-rgw":
						numRgw += int(*deployment.Spec.Replicas)
					}

					rookVersion, ok := deployment.Labels["rook-version"]
					// Some Deployments like rook-ceph-operator and rook-ceph-tools do not have "rook-version" label,
					// so skip the check of "rook-version" for such Deployments.
					// This assumes that the operator never misses labeling to the Deployments which need to be labeled.
					if ok && !strings.HasPrefix(rookVersion, expectRookVersion) {
						return fmt.Errorf("missing deployment rook version: version=%s name=%s ns=%s", rookVersion, deployment.Name, deployment.Namespace)
					}

					if deployment.Spec.Replicas == nil {
						return fmt.Errorf("deployment's spec.replicas == nil: name=%s ns=%s", deployment.Name, deployment.Namespace)
					}
					if deployment.Status.AvailableReplicas != *deployment.Spec.Replicas {
						message := fmt.Sprintf("rook's deployment's AvailableReplicas is not expected: name=%s ns=%s %d/%d",
							deployment.Name, deployment.Namespace, int(deployment.Status.AvailableReplicas), *deployment.Spec.Replicas)
						fmt.Fprintln(GinkgoWriter, message)
						return fmt.Errorf(message)
					}
				}

				if numMon != numMonExpected {
					return fmt.Errorf("number of monitors is %d, expected is %d", numMon, numMonExpected)
				}
				if numOsd != numOsdExpected {
					return fmt.Errorf("number of OSDs is %d, expected is %d", numOsd, numOsdExpected)
				}
				if numRgw != numRgwExpected {
					return fmt.Errorf("number of RGWs is %d, expected is %d", numRgw, numRgwExpected)
				}

				return nil
			}).Should(Succeed())
		})
	}
}

func getNumOsd(ns string) int {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
		"get", "cephcluster", ns, "-o", "jsonpath='{.spec.storage.storageClassDeviceSets[*].count}'")
	Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
	numOsdList := strings.Fields(string(stdout))
	numOsdExpected := 0
	for _, numOsd := range numOsdList {
		num, err := strconv.Atoi(strings.TrimSpace(string(numOsd)))
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
		numOsdExpected += num
	}
	return numOsdExpected
}

func confirmOsdPrepare() {
	for _, ns := range nss {
		numOsdExpected := getNumOsd(ns)
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "pod", "-l", "app=rook-ceph-osd-prepare", "-o=json")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			pods := new(corev1.PodList)
			err = json.Unmarshal(stdout, pods)
			Expect(err).ShouldNot(HaveOccurred(), "json=%s", stdout)

			if len(pods.Items) != numOsdExpected {
				return fmt.Errorf("number of OSD prepare pods is %d, expected is %d", len(pods.Items), numOsdExpected)
			}

			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodSucceeded {
					return fmt.Errorf("OSD prepare pod has not finished yet.")
				}

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns, "logs", pod.Name, "--tail=1")
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

				if !strings.Contains(string(stdout), "skipping OSD configuration as no devices matched") {
					continue
				}
				// log for checking
				fmt.Println("delete PVC and Job to re-create prepare Job")
				stdout, _, _ = ExecAt(boot0, "kubectl", "--namespace="+ns, "get", "pod")
				fmt.Println(string(stdout))

				pvcName := ""
				for _, volume := range pod.Spec.Volumes {
					if volume.PersistentVolumeClaim != nil && len(volume.PersistentVolumeClaim.ClaimName) != 0 {
						pvcName = volume.PersistentVolumeClaim.ClaimName
					}
				}
				Expect(pvcName).ShouldNot(Equal(""))

				stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns, "delete", "pvc", pvcName, "--wait=false")
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

				stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns, "delete", "job", "-l", "ceph.rook.io/pvc="+pvcName)
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

				stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns, "delete", "pod", "-l", "app=rook-ceph-operator")
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

				return fmt.Errorf("the osd prepare job gets failed and restarted")
			}
			return nil
		}).Should(Succeed())
	}
}

func confirmCephPod(ns string) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
		"get", "pod", "-o=json")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	pods := new(corev1.PodList)
	err = json.Unmarshal(stdout, pods)
	Expect(err).ShouldNot(HaveOccurred(), "json=%s", stdout)

	for _, pod := range pods.Items {
		// skip prepare job pods
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		// checking initContainer status of osd pod and restart if needed
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Terminated == nil {
				return fmt.Errorf("a init container of pod has not finished")
			}

			// init container finished normally
			if containerStatus.State.Terminated.ExitCode == 0 {
				continue
			}

			// return error immediately if pod is not osd
			if pod.Labels["app"] != "rook-ceph-osd" {
				return fmt.Errorf("pod status is not running: ns=%s name=%s time=%s", pod.Namespace, pod.Name, time.Now())
			}

			// log for checking
			fmt.Println("delete osd pod and re-create it")
			stdout, _, _ = ExecAt(boot0, "kubectl", "--namespace="+ns, "get", "pod")
			fmt.Println(string(stdout))

			// re-create osd pod
			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns, "delete", "pod", pod.Name)
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			return fmt.Errorf("a init container of osd pod gets failed and restarted")
		}

		// checking container status of osd pod and restart if needed
		for _, containerStatus := range pod.Status.ContainerStatuses {
			// container running or finished normally
			if containerStatus.Ready == true ||
				containerStatus.LastTerminationState.Terminated == nil ||
				containerStatus.LastTerminationState.Terminated.ExitCode == 0 {
				continue
			}

			// return error immediately if pod is not osd
			if pod.Labels["app"] != "rook-ceph-osd" {
				return fmt.Errorf("pod status is not running: ns=%s name=%s time=%s", pod.Namespace, pod.Name, time.Now())
			}

			// log for checking
			fmt.Println("delete osd pod and re-create it")
			stdout, _, _ = ExecAt(boot0, "kubectl", "--namespace="+ns, "get", "pod")
			fmt.Println(string(stdout))

			// re-create osd pod
			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns, "delete", "pod", pod.Name, "--ignore-not-found=true")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			return fmt.Errorf("an osd pod gets failed and restarted")
		}
	}

	return nil
}

func testDaemonPodsSpread(daemonName, appLabel, cephClusterNamespace string, expectDaemonCount, expectDaemonCountOnNode, expectDaemonCountInRack int) {
	By(fmt.Sprintf("checking %s Pods for %s are spread", daemonName, cephClusterNamespace), func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", "node-role.kubernetes.io/cs=true", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		nodes := new(corev1.NodeList)
		err = json.Unmarshal(stdout, nodes)
		Expect(err).ShouldNot(HaveOccurred())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+cephClusterNamespace,
			"get", "pod", "-l", appLabel, "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		pods := new(corev1.PodList)
		err = json.Unmarshal(stdout, pods)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(pods.Items).To(HaveLen(expectDaemonCount))

		nodeCounts := make(map[string]int)
		for _, pod := range pods.Items {
			nodeCounts[pod.Spec.NodeName]++
		}
		for node, count := range nodeCounts {
			Expect(count).To(Equal(expectDaemonCountOnNode), "node=%s, count=%d", node, count)
		}

		rackCounts := make(map[string]int)
		for _, node := range nodes.Items {
			if nodeCounts[node.Name] != 0 {
				rackCounts[node.Labels["topology.kubernetes.io/zone"]] += nodeCounts[node.Name]
			}
		}
		for rack, count := range rackCounts {
			Expect(count).To(Equal(expectDaemonCountInRack), "rack=%s, count=%d", rack, count)
		}
	})
}

func testMONPodsSpreadAll() {
	for _, namespace := range []string{"ceph-ssd", "ceph-object-store"} {
		testDaemonPodsSpread("MON", "app=rook-ceph-mon", namespace, 3, 1, 1)
	}
}

func testMGRPodsSpreadAll() {
	for _, namespace := range []string{"ceph-ssd", "ceph-object-store"} {
		testDaemonPodsSpread("MGR", "app=rook-ceph-mgr", namespace, 2, 1, 1)
	}
}

func testRGWPodsSpreadAll() {
	testDaemonPodsSpread("RGW", "app=rook-ceph-rgw", "ceph-object-store", 3, 1, 1)
}

func testOSDPodsSpread() {
	if doUpgrade {
		return
	}

	cephClusterName := "ceph-object-store"
	cephClusterNamespace := "ceph-object-store"
	nodeRole := "ss"
	type testTarget struct {
		nodeRole string
		device   string
	}
	for _, target := range []testTarget{
		{
			nodeRole: "cs",
			device:   "ssd",
		},
		{
			nodeRole: "ss",
			device:   "hdd",
		},
	} {
		By("checking OSD Pods for "+cephClusterName+" are spread on "+nodeRole+" nodes", func() {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", "node-role.kubernetes.io/"+target.nodeRole+"=true", "-o=json")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			nodes := new(corev1.NodeList)
			err = json.Unmarshal(stdout, nodes)
			Expect(err).ShouldNot(HaveOccurred())

			nodeCounts := make(map[string]int)
			for _, node := range nodes.Items {
				nodeCounts[node.Name] = 0
			}

			label := "app=rook-ceph-osd,ceph.rook.io/DeviceSet=" + target.device
			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+cephClusterNamespace,
				"get", "pod", "-l", label, "-o=json")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			pods := new(corev1.PodList)
			err = json.Unmarshal(stdout, pods)
			Expect(err).ShouldNot(HaveOccurred())

			for _, pod := range pods.Items {
				nodeCounts[pod.Spec.NodeName]++
			}

			var min int = math.MaxInt32
			var max int
			for _, v := range nodeCounts {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			Expect(max-min).Should(BeNumerically("<=", 1), "nodeCounts=%v", nodeCounts)

			rackCounts := make(map[string]int)
			for _, node := range nodes.Items {
				rackCounts[node.Labels["topology.kubernetes.io/zone"]] += nodeCounts[node.Name]
			}

			min = math.MaxInt32
			max = 0
			for _, v := range rackCounts {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			Expect(max-min).Should(BeNumerically("<=", 1), "rackCounts=%v", rackCounts)
		})
	}
}

func testRookRGW() {
	By("putting/getting data with s3 client", func() {
		ns := "dctest"
		waitRGW(ns, "pod-ob")

		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo 'putting getting data' > /tmp/put_get"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/put_get --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		stdout, _, _ = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd ls s3://\${BUCKET_NAME} --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(stdout).Should(ContainSubstring("put_get"))

		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/put_get /tmp/put_get_download --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "cat", "/tmp/put_get_download")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("putting getting data\n")))
	})
}

func testRookRBDAll() {
	testRookRBD("ceph-ssd-block")
}

func testRookRBD(storageClassName string) {
	pod := storageClassName + "-pod-rbd"
	By("mounting RBD of "+storageClassName, func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "mountpoint", "-d", "/test1")
			if err != nil {
				return fmt.Errorf("failed to check mount point. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		writePath := "/test1/test.txt"
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "cp", "/etc/passwd", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "sync")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "cat", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func prepareRebootRookCeph() {
	Context("preparing rook-ceph for reboot", prepareRookCeph)

	It("should store data via RGW before reboot", func() {
		ns := "dctest"
		waitRGW(ns, "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo 'reboot data' > /tmp/reboot"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/reboot --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}/reboot"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

//go:embed testdata/storage-reboot.yaml
var storageRebootYAML []byte

func testRebootRookCeph() {
	It("should get stored data via RGW after reboot", func() {
		By("recreating Pod using OBC")
		_, stderr, err := ExecAtWithInput(boot0, storageRebootYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		waitRGW("dctest", "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/reboot /tmp/reboot_download --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", "pod-ob", "--", "cat", "/tmp/reboot_download")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("reboot data\n")))
	})
}

func waitRGW(ns, podName string) {
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, podName, "--", "sh", "-c",
			`"s3cmd ls s3://\${BUCKET_NAME}/ --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}

func testRookCeph() {
	It("should be available", func() {
		testRookOperator()
		testClusterStable()
		testOSDPodsSpread()
		testMONPodsSpreadAll()
		testMGRPodsSpreadAll()
		testRGWPodsSpreadAll()
		testRookRGW()
		testRookRBDAll()
	})
}
