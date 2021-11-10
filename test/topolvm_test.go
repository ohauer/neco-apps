package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/topolvm.yaml
var topolvmYAML []byte

func prepareTopoLVM() {
	It("should prepare a Pod and a PVC", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, topolvmYAML, "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("failed to apply topolvm yaml: stdout=%s, stderr=%s", stdout, stderr)
			}
			return nil
		}).Should(Succeed())
	})
}

func testTopoLVM() {
	It("should work TopoLVM pod and auto-resizer", func() {
		By("checking the test pod is running")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n", "dctest", "pods", "topolvm-test", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get topolvm-test pod: %s: %w", stderr, err)
			}
			pod := &corev1.Pod{}
			if err := json.Unmarshal(stdout, pod); err != nil {
				return err
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type != corev1.PodReady {
					continue
				}
				if cond.Status == corev1.ConditionTrue {
					return nil
				}
			}
			return errors.New("topolvm-test pod is not ready")
		}).Should(Succeed())

		By("writing large file")
		ExecSafeAt(boot0, "kubectl", "exec", "-n", "dctest", "topolvm-test", "--", "dd", "if=/dev/zero", "of=/test1/largefile", "bs=1M", "count=110")

		By("waiting for the PV getting resized")
		Eventually(func() error {
			result, err := queryMetrics(MonitoringLargeset, `kubelet_volume_stats_capacity_bytes`)
			if err != nil {
				return err
			}

			for _, sample := range result.Data.Result {
				if sample.Metric == nil {
					continue
				}

				if string(sample.Metric["namespace"]) != "dctest" {
					continue
				}
				if string(sample.Metric["persistentvolumeclaim"]) != "topo-pvc" {
					continue
				}
				if sample.Value > (1 << 30) {
					return nil
				}

				return fmt.Errorf("filesystem capacity is under < 1 GiB: %f", float64(sample.Value))
			}

			return fmt.Errorf("no metric for PVC")
		}, 10*time.Minute).Should(Succeed())
	})
}
