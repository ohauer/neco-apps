package test

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	argoCDPasswordFile       = "./argocd-password.txt"
	ghcrDockerConfigJson     = "ghcr_dockerconfig.json"
	quayDockerConfigJson     = "quay_dockerconfig.json"
	cybozuPrivateRepoReadPAT = "cybozu_private_repo_read_pat"
)

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

//go:embed testdata/setup-teleport.yaml
var setupTeleportYAML string

const numControlPlanes = 3
const numWorkers = 6
const numNodes = numControlPlanes + numWorkers

func prepareNodes() {
	It("should increase worker nodes", func() {
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "ckecli", "cluster", "get")
			return err
		}).Should(Succeed())
		ExecSafeAt(boot0, "ckecli", "constraints", "set", "minimum-workers", strconv.Itoa(numWorkers))
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

			readyNodeSet := make(map[string]struct{})
			for _, n := range nl.Items {
				for _, c := range n.Status.Conditions {
					if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
						readyNodeSet[n.Name] = struct{}{}
					}
				}
			}
			if len(readyNodeSet) != numNodes {
				return fmt.Errorf("some nodes are not ready")
			}

			return nil
		}).Should(Succeed())
	})
}

func createNamespaceIfNotExists(ns string, privileged bool) {
	var policy string
	if privileged {
		policy = "privileged"
	}
	createNamespaceIfNotExistsWithPolicy(ns, policy)
}

// createNamespaceIfNotExistsWithPolicy creates the namespace with the policy.
// If the policy of the existing namespace is different from the specified policy, the policy of the namespace will be updated.
// If the policy argument is an empty string, the policy label will be deleted. (i.e. = default policy)
func createNamespaceIfNotExistsWithPolicy(ns string, policy string) {
	_, _, err := ExecAt(boot0, "kubectl", "get", "namespace", ns)
	if err != nil {
		ExecSafeAt(boot0, "kubectl", "create", "namespace", ns)
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "sa", "default", "-n", ns)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	}

	var labelArg string
	if policy != "" {
		labelArg = "pod-security.cybozu.com/policy=" + policy
	} else {
		labelArg = "pod-security.cybozu.com/policy-"
	}
	stdout, stderr, err := ExecAt(boot0, "kubectl", "label", "--overwrite", "namespace/"+ns, labelArg)
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
}

// testSetup tests setup of Argo CD
func testSetup() {
	It("should create secrets of account.json", func() {
		if doUpgrade {
			Skip("No need to create it when upgrading")
		}

		By("loading account.json")
		data, err := os.ReadFile("account.json")
		Expect(err).ShouldNot(HaveOccurred())

		By("creating namespace and secrets for external-dns")
		createNamespaceIfNotExists("external-dns", false)
		_, _, err = ExecAt(boot0, "kubectl", "--namespace=external-dns", "get", "secret", "clouddns")
		if err != nil {
			_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=external-dns",
				"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		}

		By("creating namespace and secrets for cert-manager")
		createNamespaceIfNotExists("cert-manager", true)
		_, _, err = ExecAt(boot0, "kubectl", "--namespace=cert-manager", "get", "secret", "clouddns")
		if err != nil {
			_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=cert-manager",
				"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		}
	})

	It("should create secrets for teleport", func() {
		if doUpgrade {
			Skip("No need to create it when upgrading")
		}

		By("creating namespace and secrets")
		createNamespaceIfNotExists("teleport", false)
		stdout, stderr, err := ExecAt(boot0, "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
			"get", "--print-value-only", "/neco/teleport/auth-token")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		teleportToken := strings.TrimSpace(string(stdout))
		teleportTmpl := template.Must(template.New("").Parse(setupTeleportYAML))
		buf := bytes.NewBuffer(nil)
		err = teleportTmpl.Execute(buf, struct {
			Token string
		}{
			Token: teleportToken,
		})
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-n", "teleport", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})

	It("should create secrets for meows", func() {
		if meowsDisabled() {
			Skip("meows is disabled")
		}
		if doUpgrade {
			Skip("No need to create it when upgrading")
		}

		By("loading meows-secret.json")
		data, err := os.ReadFile(meowsSecretFile)
		Expect(err).NotTo(HaveOccurred())
		env := make(map[string]string)
		err = json.Unmarshal(data, &env)
		Expect(err).NotTo(HaveOccurred())

		By("creating temporally file for secrets")
		fileCreateSafeAt(boot0, "slack_api_token", env["slack_api_token"])
		fileCreateSafeAt(boot0, "slack_bot_token", env["slack_bot_token"])

		By("creating meows namespace")
		createNamespaceIfNotExists("meows", false)

		By("creating secret for slack-agent in meows")
		_ = ExecSafeAt(boot0, "kubectl", "create", "secret", "generic", "slack-app-secret",
			"-n", "meows",
			"--from-file=SLACK_APP_TOKEN=slack_api_token",
			"--from-file=SLACK_BOT_TOKEN=slack_bot_token",
		)
	})

	It("should create secrets to access ghcr.io and quay.io private repositories", func() {
		if doUpgrade {
			Skip("No need to create it when upgrading")
		}

		_, err := os.Stat(ghcrDockerConfigJson)
		if err == nil {
			data, err := os.ReadFile(ghcrDockerConfigJson)
			Expect(err).ShouldNot(HaveOccurred())

			By("creating init-template namespace")
			createNamespaceIfNotExists("init-template", false)

			By("creating a secret for ghcr.io")
			_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "create", "secret", "docker-registry", "image-pull-secret-ghcr",
				"-n", "init-template",
				"--from-file=.dockerconfigjson=/dev/stdin",
			)
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)

			By("annotate secret to propagate")
			_ = ExecSafeAt(boot0, "kubectl", "annotate", "secrets", "image-pull-secret-ghcr",
				"-n", "init-template",
				"accurate.cybozu.com/propagate=create",
			)
		}

		_, err = os.Stat(quayDockerConfigJson)
		if err == nil {
			data, err := os.ReadFile(quayDockerConfigJson)
			Expect(err).ShouldNot(HaveOccurred())

			By("creating init-template namespace")
			createNamespaceIfNotExists("init-template", false)

			By("creating a secret for quay.io")
			_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "create", "secret", "docker-registry", "image-pull-secret-quay",
				"-n", "init-template",
				"--from-file=.dockerconfigjson=/dev/stdin",
			)
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)

			By("annotate secret to propagate")
			_ = ExecSafeAt(boot0, "kubectl", "annotate", "secrets", "image-pull-secret-quay",
				"-n", "init-template",
				"accurate.cybozu.com/propagate=create",
			)
		}
	})

	It("should create sandbox namespace", func() {
		createNamespaceIfNotExists("sandbox", false)
	})

	It("should create general purpose namespace for dctest", func() {
		createNamespaceIfNotExists("dctest", true)
	})

	It("should checkout neco-apps repository@"+commitID, func() {
		ExecSafeAt(boot0, "rm", "-rf", "neco-apps")

		ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128",
			"git", "clone", "https://github.com/cybozu-go/neco-apps")
		ExecSafeAt(boot0, "cd neco-apps; git checkout "+commitID)
	})

	It("should setup applications", func() {
		if !doUpgrade {
			applyNetworkPolicy()
			setupArgoCD()
		}
		if meowsDisabled() {
			ExecSafeAt(boot0, "sed", "-i", "/meows.yaml/d", "./neco-apps/argocd-config/overlays/gcp/kustomization.yaml")
		}
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/base/*.yaml")
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/overlays/"+overlayName+"/*.yaml")
		applyAndWaitForApplications(commitID)
	})

	It("should wait for rook stable", func() {
		confirmOsdPrepare()
	})

	It("should add a credential to access to cybozu-private repositories", func() {
		if doUpgrade {
			Skip("No need to create it when upgrading")
		}

		_, err := os.Stat(cybozuPrivateRepoReadPAT)
		if err == nil {
			data, err := os.ReadFile(cybozuPrivateRepoReadPAT)
			Expect(err).ShouldNot(HaveOccurred())

			By("add a credential for cybozu-private")
			_, stderr, err := ExecAtWithInput(boot0, data, "bash", "-c", "'read -sr PASSWORD; argocd repocreds add https://github.com/cybozu-private/ --username cybozu-neco --password=${PASSWORD}'")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		}
	})

	It("should set HTTP proxy", func() {
		var proxyIP string
		By("getting proxy address")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "internet-egress", "get", "svc", "squid", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %v, stderr: %v, err: %v", stdout, stderr, err)
			}

			var svc corev1.Service
			err = json.Unmarshal(stdout, &svc)
			if err != nil {
				return fmt.Errorf("stdout: %v, err: %v", stdout, err)
			}

			if len(svc.Status.LoadBalancer.Ingress) == 0 {
				return errors.New("len(svc.Status.LoadBalancer.Ingress) == 0")
			}
			proxyIP = svc.Status.LoadBalancer.Ingress[0].IP
			return nil
		}).Should(Succeed())

		proxyURL := fmt.Sprintf("http://%s:3128", proxyIP)
		ExecSafeAt(boot0, "neco", "config", "set", "node-proxy", proxyURL)
		ExecSafeAt(boot0, "neco", "config", "set", "proxy", proxyURL)

		By("waiting for docker to be restarted")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "docker", "info", "-f", "{{.HTTPProxy}}")
			if err != nil {
				return fmt.Errorf("docker info failed: %s: %w", stderr, err)
			}
			if strings.TrimSpace(string(stdout)) != proxyURL {
				return errors.New("docker has not been restarted")
			}
			return nil
		}).Should(Succeed())

		// want to do "Eventually( Consistently(<check logic>, 15sec, 1sec) )"
		By("waiting for the system to become stable")
		Eventually(func() error {
			st := time.Now()
			for {
				if time.Since(st) > 15*time.Second {
					return nil
				}
				_, _, err := ExecAt(boot0, "sabactl", "ipam", "get")
				if err != nil {
					return err
				}
				_, _, err = ExecAt(boot0, "ckecli", "cluster", "get")
				if err != nil {
					return err
				}
				time.Sleep(1 * time.Second)
			}
		}).Should(Succeed())
	})

	It("should reconfigure ignitions", func() {
		necoVersion := string(ExecSafeAt(boot0, "dpkg-query", "-W", "-f", "'${Version}'", "neco"))
		rolePaths := strings.Fields(string(ExecSafeAt(boot0, "ls", "/usr/share/neco/ignitions/roles/*/site.yml")))
		for _, rolePath := range rolePaths {
			role := strings.Split(rolePath, "/")[6]
			ExecSafeAt(boot0, "sabactl", "ignitions", "delete", role, necoVersion)
		}
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "neco", "init-data", "--ignitions-only")
			if err != nil {
				fmt.Fprintf(os.Stderr, "neco init-data failed: %s: %v\n", stderr, err)
				return fmt.Errorf("neco init-data failed: %s: %w", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should add initial user and database to grafana's MySQL", func() {
		Eventually(func() error {
			return checkStatefulSetReplicas("moco-grafana-mysql", "monitoring", 3)
		}).Should(Succeed())

		execSafeMysql := func(stmt string) {
			// Special characters including whitespaces must be escaped in ssh command execution arguments
			r := regexp.MustCompile("[^A-Za-z0-9]")
			stmt = r.ReplaceAllStringFunc(stmt, func(x string) string { return "\\" + x })
			ExecSafeAt(boot0, "kubectl", "moco", "mysql", "-u", "moco-writable", "-n", "monitoring", "grafana-mysql", "--", "-e", stmt)
		}
		// "IF NOT EXISTS" is important because these statements may be executed twice during upgrade-* workflow.
		execSafeMysql(`CREATE USER IF NOT EXISTS 'grafana'@'%' IDENTIFIED BY 'grafana'`)
		execSafeMysql(`CREATE DATABASE IF NOT EXISTS grafana`)
		execSafeMysql(`GRANT ALL ON grafana.* TO 'grafana'@'%'`)
	})

	It("exports a list of images used", func() {
		stdout := ExecSafeAt(boot0, "kubectl", "get", "pods", "-A", "-o", "jsonpath=\"{.items[*].spec.containers[*].image}\"", "|", "tr", "-s", "'[[:space:]]' ", "'\n'", "|", "sort", "-u")
		err := ioutil.WriteFile("/tmp/image_list.txt", stdout, 0644)
		Expect(err).NotTo(HaveOccurred())
	})
}

func applyAndWaitForApplications(commitID string) {
	By("creating Argo CD app")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "argocd", "app", "create", "argocd-config",
			"--upsert",
			"--repo", "https://github.com/cybozu-go/neco-apps.git",
			"--path", "argocd-config/overlays/"+overlayName,
			"--dest-namespace", "argocd",
			"--dest-server", "https://kubernetes.default.svc",
			"--sync-policy", "none",
			"--revision", commitID)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())

	// Write special process for upgrade.
	// note: do not delete this comment and By.
	By("running pre-sync special process")

	// TODO: remove this block after release the PR bellow
	// https://github.com/cybozu-go/neco-apps/pull/2248
	if doUpgrade {
		_, _, err := ExecAt(boot0, "kubectl", "get", "ns", "dev-kintone")
		if err == nil {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "annotate", "ns", "dev-kintone", "admission.cybozu.com/i-am-sure-to-delete=dev-kintone")
			Expect(err).ShouldNot(HaveOccurred(), "failed to annotate: stdout=%s, stderr=%s", stdout, stderr)
		}
	}

	// TODO: remove this block after release the PR bellow
	// https://github.com/cybozu-go/neco-apps/pull/2267
	if doUpgrade {
		_, _, err := ExecAt(boot0, "kubectl", "get", "ns", "ceph-hdd")
		if err == nil {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "annotate", "ns", "ceph-hdd", "admission.cybozu.com/i-am-sure-to-delete=ceph-hdd")
			Expect(err).ShouldNot(HaveOccurred(), "failed to annotate: stdout=%s, stderr=%s", stdout, stderr)
		}
	}

	By("syncing argocd-config")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "cd", "./neco-apps",
			"&&", "argocd", "app", "sync", "argocd-config",
			"--retry-limit", "100",
			"--local", "argocd-config/overlays/"+overlayName,
			"--async")
		if err != nil {
			return fmt.Errorf("stdout=%s, stderr=%s: %w", string(stdout), string(stderr), err)
		}
		return nil
	}).Should(Succeed())

	// Write special process for upgrade.
	// note: do not delete this comment and By.
	By("running post-sync special process")

	By("getting application list")
	stdout, _, err := kustomizeBuild("../argocd-config/overlays/" + overlayName)
	Expect(err).ShouldNot(HaveOccurred())

	type nameAndWave struct {
		name     string
		syncWave float64
	}
	var appList []nameAndWave
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		app := &unstructured.Unstructured{}
		_, _, err = decUnstructured.Decode(data, nil, app)
		Expect(err).ShouldNot(HaveOccurred())

		// Skip if the app is for tenants
		if app.GetLabels()["is-tenant"] == "true" {
			continue
		}

		wave, err := strconv.ParseFloat(app.GetAnnotations()["argocd.argoproj.io/sync-wave"], 32)
		Expect(err).ShouldNot(HaveOccurred())
		appList = append(appList, nameAndWave{name: app.GetName(), syncWave: wave})
	}
	Expect(appList).ShouldNot(HaveLen(0))

	sort.Slice(appList, func(i, j int) bool {
		if appList[i].syncWave != appList[j].syncWave {
			return appList[i].syncWave < appList[j].syncWave
		} else {
			return strings.Compare(appList[i].name, appList[j].name) <= 0
		}
	})
	fmt.Printf("application list:\n")
	for _, app := range appList {
		fmt.Printf(" %4.1f: %s\n", app.syncWave, app.name)
	}

	By("waiting initialization")
	checkAllAppsSynced := func() error {
		appStdout, stderr, err := ExecAt(boot0, "argocd", "app", "list", "-o", "json")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", appStdout, stderr, err)
		}
		var appResources []Application
		err = json.Unmarshal(appStdout, &appResources)
		if err != nil {
			return fmt.Errorf("stdout: %s, err: %v", appStdout, err)
		}
		appResourceMap := make(map[string]Application)
		for _, a := range appResources {
			appResourceMap[a.Name] = a
		}
		for _, target := range appList {
			app, ok := appResourceMap[target.name]
			if !ok {
				return fmt.Errorf("%s has not started synchronization yet", target.name)
			}

			// Skip checking since the target revision is not matching neco-apps commit id when the target is Helm.
			if app.Spec.Source.Helm == nil && app.Status.Sync.ComparedTo.Source.TargetRevision != commitID {
				return errors.New(target.name + " does not have correct target yet")
			}
			if app.Status.Sync.Status == SyncStatusCodeSynced &&
				app.Status.Health.Status == HealthStatusHealthy &&
				app.Operation == nil {
				continue
			}

			if app.Name == "rook" && app.Status.Sync.Status != SyncStatusCodeSynced && app.Operation == nil {
				fmt.Printf("%s sync rook app manually: syncStatus=%s, healthStatus=%s\n",
					time.Now().Format(time.RFC3339), app.Status.Sync.Status, app.Status.Health.Status)
				ExecAt(boot0, "argocd", "app", "sync", "rook", "--async", "--prune")
				// ignore error
			}

			// In upgrade test, syncing network-policy app may cause temporal network disruption.
			// It leads to ArgoCD's improper behavior. In spite of the network-policy app becomes Synced/Healthy, the operation does not end.
			// So terminate the unexpected operation manually in upgrade test.
			// TODO: This is workaround for ArgoCD's improper behavior. When this issue (T.B.D.) is closed, delete this block.
			if app.Status.Sync.Status == SyncStatusCodeSynced &&
				app.Status.Health.Status == HealthStatusHealthy &&
				app.Operation != nil &&
				app.Status.OperationState != nil &&
				app.Status.OperationState.Phase == "Running" {
				fmt.Printf("%s terminate unexpected operation: app=%s\n", time.Now().Format(time.RFC3339), target.name)
				stdout, stderr, err := ExecAt(boot0, "argocd", "app", "terminate-op", target.name)
				if err != nil {
					return fmt.Errorf("failed to terminate operation. app: %s, stdout: %s, stderr: %s, err: %v", target.name, stdout, stderr, err)
				}
				stdout, stderr, err = ExecAt(boot0, "argocd", "app", "sync", target.name)
				if err != nil {
					return fmt.Errorf("failed to sync application. app: %s, stdout: %s, stderr: %s, err: %v", target.name, stdout, stderr, err)
				}
			}

			err := fmt.Errorf("%s is not initialized. argocd app get %s -o json: %s", target.name, target.name, appStdout)
			if target.name == "rook" {
				namespaces := []string{"ceph-object-store", "ceph-ssd"}
				for _, namespace := range namespaces {
					stdout, stderr, kubectlerr := ExecAt(boot0, "kubectl", "get", "pod", "-n", namespace)
					err = fmt.Errorf("%v\nnamespace: %s, stdout: %s, stderr: %s, kubectlerr: %v", err, namespace, stdout, stderr, kubectlerr)
				}
			}
			return err
		}
		return nil
	}

	//TODO: remove this block after releasing
	// https://github.com/cybozu-go/neco-apps/pull/2321
	if doUpgrade {
		tenetCRDs := []string{
			"networkpolicytemplates.tenet.cybozu.io",
			"networkpolicyadmissionrules.tenet.cybozu.io",
		}
		for _, crd := range tenetCRDs {
			_, _, err := ExecAt(boot0, "kubectl", "get", "crd", crd)
			if err == nil {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "annotate", "crd", crd, fmt.Sprintf("admission.cybozu.com/i-am-sure-to-delete=%s", crd))
				Expect(err).ShouldNot(HaveOccurred(), "failed to annotate tenet crd: stdout=%s, stderr=%s", stdout, stderr)
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "crd", crd)
				Expect(err).ShouldNot(HaveOccurred(), "failed to delete tenet crd: stdout=%s, stderr=%s", stdout, stderr)
			}
		}
	}

	// want to do like "Eventually( Consistently(checkAllAppsSynced, 10sec, 1sec) )"
	Eventually(func() error {
		if doUpgrade {
			if err := removeCalicoIPTableRules(); err != nil {
				return err
			}
		}
		for i := 0; i < 10; i++ {
			err := checkAllAppsSynced()
			if err != nil {
				return err
			}
			time.Sleep(1 * time.Second)
		}
		if doUpgrade {
			if err := removeCalicoIPTableRules(); err != nil {
				return err
			}
		}
		return nil
	}, 60*time.Minute).Should(Succeed())

	// TODO: remove this block after the Calico replacement is released
	if doUpgrade {
		calicoCRDs := []string{
			"bgpconfigurations.crd.projectcalico.org",
			"bgppeers.crd.projectcalico.org",
			"blockaffinities.crd.projectcalico.org",
			"clusterinformations.crd.projectcalico.org",
			"felixconfigurations.crd.projectcalico.org",
			"globalnetworkpolicies.crd.projectcalico.org",
			"globalnetworksets.crd.projectcalico.org",
			"hostendpoints.crd.projectcalico.org",
			"ipamblocks.crd.projectcalico.org",
			"ipamconfigs.crd.projectcalico.org",
			"ipamhandles.crd.projectcalico.org",
			"ippools.crd.projectcalico.org",
			"kubecontrollersconfigurations.crd.projectcalico.org",
			"networkpolicies.crd.projectcalico.org",
			"networksets.crd.projectcalico.org",
		}
		for _, crd := range calicoCRDs {
			_, _, err := ExecAt(boot0, "kubectl", "get", "crd", crd)
			if err == nil {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "annotate", "crd", crd, fmt.Sprintf("admission.cybozu.com/i-am-sure-to-delete=%s", crd))
				Expect(err).ShouldNot(HaveOccurred(), "failed to annotate calico crd: stdout=%s, stderr=%s", stdout, stderr)
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "crd", crd)
				Expect(err).ShouldNot(HaveOccurred(), "failed to delete calico crd: stdout=%s, stderr=%s", stdout, stderr)
			}
		}
	}
}

func removeCalicoIPTableRules() error {
	stdout, stderr, err := ExecAt(boot0, "ckecli", "cluster", "get")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}
	cluster := new(ckeCluster)
	err = k8sYaml.Unmarshal(stdout, cluster)
	if err != nil {
		return err
	}
	for _, node := range cluster.Nodes {
		stdout, stderr, err = ExecAt(boot0,
			"ckecli", "ssh", node.Address, "--", "sudo", "iptables-legacy", "-L", "INPUT", "1")
		if err != nil {
			return fmt.Errorf("failed to list INPUT rule: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		if strings.HasPrefix(string(stdout), "cali-INPUT") {
			stdout, stderr, err = ExecAt(boot0,
				"ckecli", "ssh", node.Address, "--",
				"sudo", "iptables-legacy", "-D", "INPUT", "-m", "comment", "--comment", "\"cali:Cz_u1IQiXIMmKD4c\"", "-j", "cali-INPUT")
			if err != nil {
				return fmt.Errorf("failed to remove cali-INPUT rule: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
		}

		stdout, stderr, err = ExecAt(boot0,
			"ckecli", "ssh", node.Address, "--", "sudo", "iptables-legacy", "-L", "OUTPUT", "1")
		if err != nil {
			return fmt.Errorf("failed to list OUTPUT rule: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		if strings.HasPrefix(string(stdout), "cali-OUTPUT") {
			stdout, stderr, err = ExecAt(boot0,
				"ckecli", "ssh", node.Address, "--",
				"sudo", "iptables-legacy", "-D", "OUTPUT", "-m", "comment", "--comment", "\"cali:tVnHkvAo15HuiPy0\"", "-j", "cali-OUTPUT")
			if err != nil {
				return fmt.Errorf("failed to remove cali-OUTPUT rule: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
		}

		stdout, stderr, err = ExecAt(boot0,
			"ckecli", "ssh", node.Address, "--", "sudo", "iptables-legacy", "-L", "FORWARD", "1")
		if err != nil {
			return fmt.Errorf("failed to list FORWARD rule: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		if strings.HasPrefix(string(stdout), "cali-FORWARD") {
			stdout, stderr, err = ExecAt(boot0,
				"ckecli", "ssh", node.Address, "--",
				"sudo", "iptables-legacy", "-D", "FORWARD", "-m", "comment", "--comment", "\"cali:wUHhoiAYhphO9Mso\"", "-j", "cali-FORWARD")
			if err != nil {
				return fmt.Errorf("failed to remove cali-FORWARD rules: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
		}
	}
	return nil
}

// Sometimes synchronization fails when argocd applies network policies.
// So, apply the network policies before argocd synchronization.
// TODO: This is a workaround. When this issue is solved, delete this func.
func applyNetworkPolicy() {
	By("apply namespaces")
	namespaceManifest, stderr, err := kustomizeBuild("../namespaces/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)

	stdout, stderr, err := ExecAtWithInput(boot0, namespaceManifest, "kubectl", "apply", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "failed to apply namespaces: stdout=%s, stderr=%s", stdout, stderr)

	stdout, stderr, err = ExecAt(boot0, "kubectl", "apply", "-f", "./neco-apps/customer-egress/base/namespace.yaml")
	Expect(err).ShouldNot(HaveOccurred(), "failed to apply customer-egress namespace: stdout=%s, stderr=%s", stdout, stderr)

	By("apply network-policies")
	netpolManifest, stderr, err := kustomizeBuild("../network-policy/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)

	var nonCRDResources []*unstructured.Unstructured
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(netpolManifest)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		resources := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(data, nil, resources)
		if err != nil {
			continue
		}
		if gvk.Kind != "CustomResourceDefinition" {
			nonCRDResources = append(nonCRDResources, resources)
			continue
		}

		stdout, stderr, err = ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply crd: stdout=%s, stderr=%s", stdout, stderr)
	}

	for _, r := range nonCRDResources {
		labels := r.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		// ArgoCD will add this label, so adding this label here beforehand to speed up CI
		labels["app.kubernetes.io/instance"] = "network-policy"
		r.SetLabels(labels)
		data, err := r.MarshalJSON()
		Expect(err).ShouldNot(HaveOccurred(), "failed to marshal json. err=%s", err)
		stdout, stderr, err = ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply non-crd resource: stdout=%s, stderr=%s", stdout, stderr)
	}
}

func setupArgoCD() {
	By("installing Argo CD")
	createNamespaceIfNotExists("argocd", true)
	data, err := os.ReadFile("install.yaml")
	Expect(err).ShouldNot(HaveOccurred())
	_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-n", "argocd", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "faied to apply install.yaml. stderr=%s", stderr)

	By("waiting Argo CD comes up")
	var podList corev1.PodList
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "argocd",
			"-l", "app.kubernetes.io/name=argocd-server", "-o", "json")
		if err != nil {
			return fmt.Errorf("unable to get argocd-server pods. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		err = json.Unmarshal(stdout, &podList)
		if err != nil {
			return err
		}
		if podList.Items == nil {
			return errors.New("podList.Items is nil")
		}
		if len(podList.Items) != 1 {
			return fmt.Errorf("podList.Items is not 1: %d", len(podList.Items))
		}
		return nil
	}).Should(Succeed())

	By("getting Argo CD admin password from Secret")
	var password string
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "secret", "-n", "argocd", "argocd-initial-admin-secret", "-o", "json")
		if err != nil {
			return fmt.Errorf("unable to get argocd Secret. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		secret := new(corev1.Secret)
		err = json.Unmarshal(stdout, secret)
		if err != nil {
			return err
		}

		p, ok := secret.Data["password"]
		if !ok {
			return errors.New("password not found in argocd Secret")
		}
		password = string(p)
		return nil
	}).Should(Succeed())
	saveArgoCDPassword(password)
	ExecSafeAt(boot0, "kubectl", "delete", "secret", "-n", "argocd", "argocd-initial-admin-secret")

	By("getting node address")
	var nodeList corev1.NodeList
	data = ExecSafeAt(boot0, "kubectl", "get", "nodes", "-o", "json")
	err = json.Unmarshal(data, &nodeList)
	Expect(err).ShouldNot(HaveOccurred(), "data=%s", string(data))
	Expect(nodeList.Items).ShouldNot(BeEmpty())
	node := nodeList.Items[0]

	var nodeAddress string
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeInternalIP {
			continue
		}
		nodeAddress = addr.Address
	}
	Expect(nodeAddress).ShouldNot(BeNil())

	By("getting node port")
	var svc corev1.Service
	data = ExecSafeAt(boot0, "kubectl", "get", "svc/argocd-server", "-n", "argocd", "-o", "json")
	err = json.Unmarshal(data, &svc)
	Expect(err).ShouldNot(HaveOccurred(), "data=%s", string(data))
	Expect(svc.Spec.Ports).ShouldNot(BeEmpty())

	var nodePort string
	for _, port := range svc.Spec.Ports {
		if port.Name != "http" {
			continue
		}
		nodePort = strconv.Itoa(int(port.NodePort))
	}
	Expect(nodePort).ShouldNot(BeNil())

	By("logging in to Argo CD")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "argocd", "login", nodeAddress+":"+nodePort,
			"--insecure", "--username", "admin", "--password", loadArgoCDPassword())
		if err != nil {
			return fmt.Errorf("failed to login to argocd. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}
