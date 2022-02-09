package test

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

var (
	bastionPushgatewayFQDN = testID + "-pushgateway-bastion.gcp0.dev-ne.co"
	forestPushgatewayFQDN  = testID + "-pushgateway-forest.gcp0.dev-ne.co"
)

var (
	grafanaFQDN = testID + "-grafana.gcp0.dev-ne.co"
)

func testMachinesEndpoints() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "cronjob/machines-endpoints-cronjob")
			if err != nil {
				return err
			}

			return nil
		}).Should(Succeed())
	})

	It("should register endpoints", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "endpoints/prometheus-node-targets", "-o=json")
			if err != nil {
				return err
			}

			endpoints := new(corev1.Endpoints)
			err = json.Unmarshal(stdout, endpoints)
			if err != nil {
				return err
			}

			if len(endpoints.Subsets) != 1 {
				return errors.New("len(endpoints.Subsets) != 1")
			}
			if len(endpoints.Subsets[0].Addresses) == 0 {
				return errors.New("no address in endpoints")
			}
			if len(endpoints.Subsets[0].Ports) == 0 {
				return errors.New("no port in endpoints")
			}

			return nil
		}).Should(Succeed())
	})
}

func testKubeStateMetrics() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("kube-state-metrics", "kube-system", 2)
		}).Should(Succeed())
	})

	It("should export specified labels and annotations", func() {
		expectedMetricsLabels := map[string]map[string]map[string]bool{
			// kind -> label/annotation -> its name -> true
			"namespace": {
				"label": {
					"team":        true,
					"development": true,
				},
			},
			"node": {
				"label": {
					"cke.cybozu.com/rack":   true,
					"cke.cybozu.com/role":   true,
					"cke.cybozu.com/master": true,
				},
			},
			"persistentvolumeclaim": {
				"annotation": {
					"volume.kubernetes.io/selected-node": true,
				},
			},
		}

		Eventually(func() error {
			result, err := queryMetrics(MonitoringLargeset, `{__name__=~"kube_.*_(labels|annotations)"}`)
			if err != nil {
				return err
			}

			metricsLabels := map[string]map[string]bool{}
			for _, sample := range result.Data.Result {
				if sample.Metric == nil {
					continue
				}

				name := string(sample.Metric["__name__"])
				var la string
				if strings.HasSuffix(name, "_labels") {
					name = name[5 : len(name)-len("_labels")]
					la = "label"
				} else {
					name = name[5 : len(name)-len("_annotations")]
					la = "annotation"
				}
				if _, ok := metricsLabels[name]; !ok {
					metricsLabels[name] = map[string]bool{}
				}

				for labelName := range sample.Metric {
					label := string(labelName)
					if strings.HasPrefix(label, la+"_") {
						metricsLabels[name][label] = true
					}
				}
			}

			r := regexp.MustCompile("[^a-zA-Z]")
			for kind, x := range expectedMetricsLabels {
				for la, y := range x {
					for name := range y {
						name = r.ReplaceAllString(name, "_")
						if _, ok := metricsLabels[kind][la+"_"+name]; !ok {
							return fmt.Errorf("kube_%s_%ss metrics does not have %s_%s label", kind, la, la, name)
						}
					}
				}
			}

			return nil
		}, 10*time.Minute).Should(Succeed())
	})
}

//go:embed testdata/monitoring-pushgateway.yaml
var monitoringPushgatewayYAML string

func preparePushgateway() {
	It("should create HTTPProxy for Pushgateway", func() {
		tmpl := template.Must(template.New("").Parse(monitoringPushgatewayYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testPushgateway() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("pushgateway", "monitoring", 1)
		}).Should(Succeed())
	})

	It("should be accessed from Bastion", func() {
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", bastionPushgatewayFQDN+":80:"+ip, "-s", "http://"+bastionPushgatewayFQDN+"/-/healthy", "-o", "/dev/null")
			if err != nil {
				log.Warn("curl failed", map[string]interface{}{
					"stdout": string(stdout),
					"stderr": string(stderr),
				})
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", string(stdout), string(stderr), err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should be accessed from Forest", func() {
		var forestIP string
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-forest", "envoy")
			if err != nil {
				return err
			}
			forestIP = ip
			return nil
		}).Should(Succeed())
		Eventually(func() error {
			return exec.Command("ip", "netns", "exec", "external", "curl", "--resolve", forestPushgatewayFQDN+":80:"+forestIP, forestPushgatewayFQDN+"/-/healthy", "-m", "5").Run()
		}).Should(Succeed())
	})
}

func getLoadBalancerIP(namespace, service string) (string, error) {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", namespace, "get", "service", service, "-o=json")
	if err != nil {
		return "", fmt.Errorf("unable to get %s/%s. stdout: %s, stderr: %s, err: %w", namespace, service, stdout, stderr, err)
	}
	svc := new(corev1.Service)
	err = json.Unmarshal(stdout, svc)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal %s/%s. err: %w", namespace, service, err)
	}
	if len(svc.Status.LoadBalancer.Ingress) != 1 {
		return "", fmt.Errorf("len(svc.Status.LoadBalancer.Ingress) != 1. %d", len(svc.Status.LoadBalancer.Ingress))
	}
	return svc.Status.LoadBalancer.Ingress[0].IP, nil
}

//go:embed testdata/monitoring-grafana-operator.yaml
var monitoringGrafanaYAML string

func prepareGrafanaOperator() {
	It("should create HTTPProxy for grafana", func() {
		tmpl := template.Must(template.New("").Parse(monitoringGrafanaYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "failed to create HTTPProxy. stderr: %s", stderr)
	})
}

func testGrafanaOperator() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("grafana-deployment", "monitoring", 2)
		}).Should(Succeed())

		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("grafana-test", "monitoring")
		}).Should(Succeed())
	})

	It("should have data sources and dashboards", func() {
		By("getting admin stats from grafana")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", grafanaFQDN+":443:"+ip, "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", "https://"+grafanaFQDN+"/api/admin/stats", "-m", "5")
			if err != nil {
				return fmt.Errorf("unable to get admin stats, stderr: %s, err: %v", stderr, err)
			}
			var adminStats struct {
				Dashboards  int `json:"dashboards"`
				Datasources int `json:"datasources"`
			}
			err = json.Unmarshal(stdout, &adminStats)
			if err != nil {
				return err
			}
			if adminStats.Datasources == 0 {
				return fmt.Errorf("no data sources")
			}
			if adminStats.Dashboards == 0 {
				return fmt.Errorf("no dashboards")
			}
			return nil
		}).Should(Succeed())

		By("confirming all dashboards are successfully registered")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", grafanaFQDN+":443:"+ip, "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", "https://"+grafanaFQDN+"/api/search?type=dash-db")
			if err != nil {
				return fmt.Errorf("unable to get dashboards, stderr: %s, err: %v", stderr, err)
			}
			var dashboards []struct {
				ID int `json:"id"`
			}
			err = json.Unmarshal(stdout, &dashboards)
			if err != nil {
				return err
			}

			// NOTE: expectedNum is the number of files under monitoring/base/grafana/dashboards
			if len(dashboards) != numGrafanaDashboard {
				return fmt.Errorf("len(dashboards) should be %d: %d", numGrafanaDashboard, len(dashboards))
			}
			return nil
		}).Should(Succeed())
	})
}

func testVictoriaMetricsOperator() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("victoriametrics-operator", "monitoring", 2)
		}).Should(Succeed())
	})
}

type vmSetType struct {
	small        bool
	name         string
	vmamCount    int
	vmagentCount int
	vmalertCount int
}

// shrinked version of VMRule
type VMRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		Groups []struct {
			Name  string `json:"name"`
			Rules []struct {
				Alert  string `json:"alert"`
				Record string `json:"record"`
			} `json:"rules"`
		} `json:"groups"`
	} `json:"spec"`
}

// shrinked version of result of vmalert /api/v1/groups API
type VMAlertAPIV1GroupsResult struct {
	Data struct {
		Groups []struct {
			Name          string `json:"name"`
			AlertingRules []struct {
				Name string `json:"name"`
			} `json:"alerting_rules"`
			RecordingRules []struct {
				Name string `json:"name"`
			} `json:"recording_rules"`
		} `json:"groups"`
	} `json:"data"`
}

func testVMCommonClusterComponents(setType vmSetType) {
	It("should be deployed successfully (vmalertmanager)", func() {
		Eventually(func() error {
			return checkStatefulSetReplicas("vmalertmanager-vmalertmanager-"+setType.name, "monitoring", setType.vmamCount)
		}).Should(Succeed())
	})

	It("should reply successfully (vmalertmanager)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalertmanager,app.kubernetes.io/instance=vmalertmanager-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmamCount {
				return errors.New("vmalertmanager pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "--", "curl", "http://localhost:9093/-/healthy")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:9093/-/healthy, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmalert)", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("vmalert-vmalert-"+setType.name, "monitoring", setType.vmalertCount)
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmagent)", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("vmagent-vmagent-"+setType.name, "monitoring", setType.vmagentCount)
		}).Should(Succeed())
	})

	It("should reply successfully (vmalert)", func() {
		By("reading VMRules")
		expectedAlerts := []string{}
		expectedRecords := []string{}
		err := filepath.Walk("../monitoring/base/victoriametrics/rules", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := k8sYaml.NewYAMLReader(bufio.NewReader(file))
			for {
				data, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("failed to read yaml: %v", err)
				}
				var r VMRule
				yaml.Unmarshal(data, &r)
				if r.Kind != "VMRule" {
					continue
				}
				if setType.small && r.Labels["smallset"] != "true" {
					continue
				}
				for _, group := range r.Spec.Groups {
					for _, rule := range group.Rules {
						if len(rule.Alert) != 0 {
							expectedAlerts = append(expectedAlerts, rule.Alert)
						} else if len(rule.Record) != 0 {
							expectedRecords = append(expectedRecords, rule.Record)
						}
					}
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		sort.Strings(expectedAlerts)
		sort.Strings(expectedRecords)

		By("checking vmalerts")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalert,app.kubernetes.io/instance=vmalert-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmalertCount {
				return errors.New("vmalert pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "--", "curl", "http://localhost:8080/api/v1/groups")
				if err != nil {
					return fmt.Errorf("unable to curl :8080/api/v1/groups, stderr: %s, err: %v", stderr, err)
				}
				var r VMAlertAPIV1GroupsResult
				err = json.Unmarshal(stdout, &r)
				if err != nil {
					return err
				}
				actualAlerts := []string{}
				actualRecords := []string{}
				for _, group := range r.Data.Groups {
					for _, rule := range group.AlertingRules {
						if len(rule.Name) != 0 {
							actualAlerts = append(actualAlerts, rule.Name)
						}
					}
					for _, rule := range group.RecordingRules {
						if len(rule.Name) != 0 {
							actualRecords = append(actualRecords, rule.Name)
						}
					}
				}
				sort.Strings(actualAlerts)
				sort.Strings(actualRecords)
				if !reflect.DeepEqual(actualAlerts, expectedAlerts) {
					return fmt.Errorf("vmalert does not load all rules actualAlerts=%v, expectedAlerts=%v",
						actualAlerts, expectedAlerts)
				}
				if !reflect.DeepEqual(actualRecords, expectedRecords) {
					return fmt.Errorf("vmalert does not load all rules actualRecords=%v, expectedRecords=%v",
						actualRecords, expectedRecords)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should find endpoint (vmagent)", func() {
		By("reading scraping resources")
		jobNames := []string{}
		err := filepath.Walk("../monitoring/base/victoriametrics/rules", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := k8sYaml.NewYAMLReader(bufio.NewReader(file))
			for {
				data, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("failed to read yaml: %v", err)
				}
				var metaType string
				var r VMScrapeOrRule
				yaml.Unmarshal(data, &r)
				var relabelConfigs [][]RelabelConfig
				switch r.Kind {
				case "VMServiceScrape":
					metaType = "service"
					for _, ep := range r.Spec.ServiceScrapeEndpoints {
						relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
					}
				case "VMPodScrape":
					metaType = "pod"
					for _, ep := range r.Spec.PodScrapeEndpoints {
						relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
					}
				case "VMNodeScrape":
					metaType = "node"
					relabelConfigs = append(relabelConfigs, r.Spec.NodeScrapeRelabelConfigs)
				case "VMProbe":
				default:
					continue
				}
				if setType.small && r.Labels["smallset"] != "true" {
					continue
				}

				metaLabelPrefix := "__meta_kubernetes_" + metaType + "_label_"
				for _, rcs := range relabelConfigs {
					for _, rc := range rcs {
						if rc.Action == "" && rc.TargetLabel == "job" && rc.Replacement != "" && !strings.Contains(rc.Replacement, "/") {
							if !strings.HasSuffix(rc.Replacement, "$1") {
								jobNames = append(jobNames, rc.Replacement)
								continue
							}

							fmt.Printf("found job replacement %s\n", rc.Replacement)

							if rc.Regex != "" || len(rc.SourceLabels) != 1 || !strings.HasPrefix(rc.SourceLabels[0], metaLabelPrefix) || len(r.Spec.Selector.MatchLabels) != 0 {
								fmt.Printf("not match\n")
								continue
							}
							labelName := strings.TrimPrefix(rc.SourceLabels[0], metaLabelPrefix)
							fmt.Printf("labelName = %s\n", labelName)
							jobNamePrefix := strings.TrimSuffix(rc.Replacement, "$1")
							fmt.Printf("jobNamePrefix = %s\n", jobNamePrefix)
							for _, me := range r.Spec.Selector.MatchExpressions {
								if me.Key == labelName && me.Operator == metav1.LabelSelectorOpIn {
									fmt.Printf("found matchExpression element\n")
									for _, label := range me.Values {
										jobNames = append(jobNames, jobNamePrefix+label)
									}
								}
							}
						}
					}
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("checking vmagents")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmagentCount {
				return errors.New("vmagent pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					"-c", "vmagent", podName, "--",
					"curl", "http://localhost:8429/api/v1/targets")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8429/api/v1/targets, stderr: %s, err: %v", podName, stderr, err)
				}

				var response struct {
					TargetsResult promv1.TargetsResult `json:"data"`
				}
				err = json.Unmarshal(stdout, &response)
				if err != nil {
					return err
				}

				const stoppedMachinesInDCTest = 1
				downedMonitorHW := 0
				for _, jobName := range jobNames {
					// When meows is disabled, the target pod for these jobs does not exist.
					if (jobName == "meows" || jobName == "meows-runner") && meowsDisabled() {
						continue
					}

					targets := findTargets(string(jobName), response.TargetsResult.Active)
					if len(targets) == 0 {
						return fmt.Errorf("target is not found, job_name: %s", jobName)
					}
					for _, target := range targets {
						if target.Health != promv1.HealthGood {
							if target.Labels["job"] != "monitor-hw" {
								return fmt.Errorf("target is not 'up', job_name: %s, health: %s", jobName, target.Health)
							}
							downedMonitorHW++
							if downedMonitorHW > stoppedMachinesInDCTest {
								return fmt.Errorf("too many monitor-hw jobs are down; health: %s", target.Health)
							}
						}
					}
				}
			}
			return nil
		}).Should(Succeed())
	})

}

func testVMSmallsetClusterComponents() {
	testVMCommonClusterComponents(vmSetType{
		small:        true,
		name:         "smallset",
		vmamCount:    1,
		vmalertCount: 1,
		vmagentCount: 1,
	})

	It("should be deployed successfully (vmsingle)", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("vmsingle-vmsingle-smallset", "monitoring", 1)
		}).Should(Succeed())
	})

	It("should reply successfully (vmsingle)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmsingle,app.kubernetes.io/instance=vmsingle-smallset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("vmsingle pod doesn't exist")
			}
			podName := podList.Items[0].Name

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "--", "curl", "http://localhost:8429/api/v1/labels")
			if err != nil {
				return fmt.Errorf("unable to curl :8429/api/v1/labels, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}

func testVMLargesetClusterComponents() {
	const vmstorageCount = 3
	const vmselectCount = 3
	const vminsertCount = 3

	testVMCommonClusterComponents(vmSetType{
		small:        false,
		name:         "largeset",
		vmamCount:    3,
		vmalertCount: 3,
		vmagentCount: 3,
	})

	It("should be deployed successfully (vmstorage)", func() {
		Eventually(func() error {
			return checkStatefulSetReplicas("vmstorage-vmcluster-largeset", "monitoring", vmstorageCount)
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmselect)", func() {
		Eventually(func() error {
			return checkStatefulSetReplicas("vmselect-vmcluster-largeset", "monitoring", vmselectCount)
		}).Should(Succeed())
	})

	It("should be deployed successfully (vminsert)", func() {
		Eventually(func() error {
			return checkDeploymentReplicas("vminsert-vmcluster-largeset", "monitoring", vminsertCount)
		}).Should(Succeed())
	})

	It("should reply successfully (vmselect)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmselect,app.kubernetes.io/instance=vmcluster-largeset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != vmselectCount {
				return errors.New("vmselect pod count mistatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "--", "curl", "http://localhost:8481/select/0/prometheus/api/v1/labels")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8429/select/0/prometheus/api/v1/labels, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})
}

func findTargets(job string, targets []promv1.ActiveTarget) []*promv1.ActiveTarget {
	ret := []*promv1.ActiveTarget{}
	for _, t := range targets {
		if string(t.Labels["job"]) == job {
			ret = append(ret, &t)
		}
	}
	return ret
}

func checkAlerts() {
	It("check alerts", func() {
		Eventually(func() error {
			stdout := ExecSafeAt(boot0, "kubectl", "exec", "-n", "monitoring",
				"deployment/vmalert-vmalert-largeset", "-c", "vmalert", "--",
				"curl", "-s", "http://localhost:8080/api/v1/alerts")

			type Alert struct {
				Id          string            `json:"id"`
				Name        string            `json:"name"`
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
			}
			var result struct {
				Data struct {
					Alerts []Alert `json:"alerts"`
				} `json:"data"`
				Status string `json:"status"`
			}
			err := json.Unmarshal(stdout, &result)
			if err != nil {
				return err
			}
			if result.Status != "success" {
				return fmt.Errorf("status not success")
			}

			alerts := []Alert{}
			for _, a := range result.Data.Alerts {
				switch a.Labels["alertname"] {
				// CKE etcd is not backed up on gcp/placemat environment.
				// (see cybozu-go/neco/debian/lib/systemd/system/backup-cke-etcd.service)
				case "CKEEtcdBackupNotSucceeded":
					continue
				case "CKEEtcdBackupNotExecuted":
					continue

				// kube-storage-version-migrator migrates all resources on its first deployment.
				// This migration takes much time and it is not a problem in itself.
				case "KubeStorageVersionMigratorRunning":
					continue
				}

				alerts = append(alerts, a)
			}

			if len(alerts) != 0 {
				out, err := json.MarshalIndent(alerts, "", "  ")
				if err != nil {
					return err
				}
				return fmt.Errorf("alerts still exist: \n%s", string(out))
			}

			return nil
		}, time.Minute*20 /* shorter timeout than normal */).Should(Succeed())
	})
}
