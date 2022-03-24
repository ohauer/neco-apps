package test

import (
	_ "embed"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/psa-hostnetwork-pod.yaml
var psaHostNetworkPodYAML []byte

//go:embed testdata/psa-hostpath-pod.yaml
var psaHostPathPodYAML []byte

//go:embed testdata/psa-root-pod.yaml
var psaRunAsRootPodYAML []byte

type PSATestCase struct {
	Description      string
	Manifest         []byte
	PolicyAndResults map[string]bool // policy name -> allowed or not
}

var psaTestCases = []PSATestCase{
	{
		"which uses hostNetwork",
		psaHostNetworkPodYAML,
		map[string]bool{
			"baseline":  false,
			"traceable": false,
			"rootable":  false,
		},
	},
	{
		"which uses /sys/kernel/tracing hostPath",
		psaHostPathPodYAML,
		map[string]bool{
			"baseline":  false,
			"traceable": true,
			"rootable":  true,
		},
	},
	{
		"which runs as root",
		psaRunAsRootPodYAML,
		map[string]bool{
			"baseline":  false,
			"traceable": false,
			"rootable":  true,
		},
	},
}

func preparePodSecurityAdmission() {
	It("should create namepaces for psa", func() {
		for _, c := range psaTestCases {
			for policy := range c.PolicyAndResults {
				createNamespaceIfNotExistsWithPolicy("psa-"+policy, policy)
			}
		}
	})
}

func testPodSecurityAdmission() {
	It("should be configured properly", func() {
		for _, c := range psaTestCases {
			for policy, result := range c.PolicyAndResults {
				var verb string
				if result {
					verb = "accepted"
				} else {
					verb = "prohibited"
				}
				By(fmt.Sprintf("checking that a pod %s will be %s in %s policy", c.Description, verb, policy))
				_, stderr, err := ExecAtWithInput(boot0, c.Manifest, "kubectl", "apply", "-n", "psa-"+policy, "-f", "-")
				if result {
					Expect(err).NotTo(HaveOccurred(), "stderr: %s, err: %v", stderr, err)
				} else {
					Expect(err).To(HaveOccurred())
					Expect(string(stderr)).Should(ContainSubstring(`admission webhook "`+policy+`.vpod.kb.io" denied the request`), "stderr: %s", stderr)
				}
			}
		}
	})
}
