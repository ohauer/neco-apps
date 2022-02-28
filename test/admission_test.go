package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

//go:embed testdata/admission-pod-bad-image.yaml
var admissionPodBadImageYAML []byte

//go:embed testdata/admission-pod-ephemeral-storage.yaml
var admissionPodEphemeralStorageLimitYAML []byte

//go:embed testdata/admission-networkpolicy.yaml
var admissionNetworkPolicyYAML []byte

//go:embed testdata/admission-httpproxy-bad.yaml
var admissionHTTPProxyBadYAML []byte

//go:embed testdata/admission-httpproxy-bad-bastion.yaml
var admissionHTTPProxyBadBastionYAML []byte

//go:embed testdata/admission-httpproxy-annotated.yaml
var admissionHTTPProxyAnnotatedYAML []byte

//go:embed testdata/admission-application.yaml
var admissionApplicationYAML string

func testAdmission() {
	It("should validate Calico NetworkPolicy", func() {
		_, stderr, err := ExecAtWithInput(boot0, admissionNetworkPolicyYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "vnetworkpolicy.kb.io" denied the request`))
	})

	It("should default/validate Contour HTTPProxy", func() {
		By("creating HTTPProxy with annotations")
		stdout, stderr, err := ExecAtWithInput(boot0, admissionHTTPProxyAnnotatedYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "-n", "default", "httpproxy/annotated", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		hp := &unstructured.Unstructured{}
		err = json.Unmarshal(stdout, hp)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, err: %v", stdout, err)
		Expect(hp.GetAnnotations()).To(HaveKeyWithValue("kubernetes.io/ingress.class", "bastion"))

		By("updating HTTPProxy to remove annotations")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "annotate", "-n", "default", "httpproxy/annotated", "kubernetes.io/ingress.class-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		stdout, stderr, err = ExecAtWithInput(boot0, admissionHTTPProxyAnnotatedYAML, "kubectl", "delete", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		By("creating HTTPProxy without annotations nor a field")
		stdout, stderr, err = ExecAtWithInput(boot0, admissionHTTPProxyBadYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "-n", "default", "httpproxy/bad", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		hp = &unstructured.Unstructured{}
		err = json.Unmarshal(stdout, hp)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, err: %v", stdout, err)
		ingress, ok, err := unstructured.NestedString(hp.UnstructuredContent(), "spec", "ingressClassName")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, err: %v", stdout, err)
		Expect(ok).To(BeTrue())
		Expect(ingress).To(Equal("forest"))

		By("updating HTTPProxy to change ingressClassName field")
		stdout, stderr, err = ExecAtWithInput(boot0, admissionHTTPProxyBadBastionYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})

	It("should validate Application", func() {
		By("creating Application which points to neco-apps repo and belongs to default project")
		tmpl := template.Must(template.New("").Parse(admissionApplicationYAML))
		type tmplParams struct {
			Name    string
			Project string
			URL     string
		}
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, tmplParams{"valid", "default", "https://github.com/cybozu-go/neco-apps.git"})
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		ExecSafeAt(boot0, "kubectl", "delete", "application", "valid", "-n", "argocd")

		By("denying to create Application which points to invalid repo and belongs to default project")
		buf.Reset()
		err = tmpl.Execute(buf, tmplParams{"invalid", "default", "https://github.com/cybozu-go/invalid-apps.git"})
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should validate deletion", func() {
		By("trying to delete a namespace")
		_, _, err := ExecAt(boot0, "kubectl", "delete", "namespace", "internet-egress")
		Expect(err).Should(HaveOccurred())

		By("trying to delete a CRD")
		_, _, err = ExecAt(boot0, "kubectl", "delete", "crd", "applications.argoproj.io")
		Expect(err).Should(HaveOccurred())

		By("trying to delete a CephCluster")
		_, _, err = ExecAt(boot0, "kubectl", "delete", "-n", "ceph-object-store", "cephclusters.ceph.rook.io", "ceph-object-store")
		Expect(err).Should(HaveOccurred())
	})

	It("should validate pod", func() {
		_, stderr, err := ExecAtWithInput(boot0, admissionPodBadImageYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring(`admission webhook "vpod.kb.io" denied the request`))
	})

	It("should mutate pod to apply ephemeral storage limitation", func() {
		stdout, stderr, err := ExecAtWithInput(boot0, admissionPodEphemeralStorageLimitYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		By("confirming that resource request/limit of ephemeral storage are added/overwritten")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pod", "pod-mutator-test", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		po := new(corev1.Pod)
		err = json.Unmarshal(stdout, po)
		Expect(err).NotTo(HaveOccurred())

		containers := po.Spec.Containers
		containers = append(containers, po.Spec.InitContainers...)

		// assumed that containers[0]'s request/limit of ephemeral storage are not set originally and added by admission.
		Expect(containers[0].Resources.Requests).ShouldNot(BeNil())
		Expect(containers[0].Resources.Limits).ShouldNot(BeNil())
		ephemeralRequest, ok := containers[0].Resources.Requests[corev1.ResourceEphemeralStorage]
		Expect(ok).Should(BeTrue())
		ephemeralLimit, ok := containers[0].Resources.Limits[corev1.ResourceEphemeralStorage]
		Expect(ok).Should(BeTrue())

		for _, con := range containers {
			Expect(con.Resources.Requests).ShouldNot(BeNil())
			Expect(con.Resources.Limits).ShouldNot(BeNil())
			Expect(con.Resources.Requests[corev1.ResourceEphemeralStorage]).To(Equal(ephemeralRequest))
			Expect(con.Resources.Limits[corev1.ResourceEphemeralStorage]).To(Equal(ephemeralLimit))
		}
	})
}
