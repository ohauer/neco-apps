package test

import (
	_ "embed"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/psa-hostnetwork-pod.yaml
var psaHostNetworkPodYAML []byte

//go:embed testdata/psa-hostpath-pod.yaml
var psaHostPathPodYAML []byte

func preparePodSecurityAdmission() {
	It("should create namepaces for psa", func() {
		createNamespaceIfNotExistsWithPolicy("psa-baseline", "baseline")
		createNamespaceIfNotExistsWithPolicy("psa-traceable", "traceable")
	})
}

func testPodSecurityAdmission() {
	It("should prohibit pod that uses hostNetwork in baseline policy", func() {
		_, stderr, err := ExecAtWithInput(boot0, psaHostNetworkPodYAML, "kubectl", "apply", "-n", "psa-baseline", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "baseline.vpod.kb.io" denied the request`))
	})

	It("should accept pod that uses /sys/kernel/tracing hostPath in traceable policy", func() {
		_, _, err := ExecAtWithInput(boot0, psaHostPathPodYAML, "kubectl", "apply", "-n", "psa-traceable", "-f", "-")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should prohibit pod that uses /sys/kernel/tracing hostPath in baseline policy", func() {
		_, stderr, err := ExecAtWithInput(boot0, psaHostPathPodYAML, "kubectl", "apply", "-n", "psa-baseline", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "baseline.vpod.kb.io" denied the request`))
	})
}
