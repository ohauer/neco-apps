package test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testSessionLog() {
	const s3gwBucketEndpoint = "http://s3gw.session-log.svc/bucket/"

	type bucketListResult struct {
		Objects []struct {
			Key  string `json:"key"`
			Size int    `json:"size"`
		} `json:"objects"`
	}

	It("check object bucket", func() {
		const objectKey = "dctest-cat"
		const s3gwObjectURL = s3gwBucketEndpoint + objectKey

		checkObjectKey := func(expect bool) {
			stdout, _, err := ExecAt(boot0, "curl", "-sf", s3gwBucketEndpoint)
			Expect(err).ShouldNot(HaveOccurred())
			var result bucketListResult
			err = json.Unmarshal(stdout, &result)
			Expect(err).ShouldNot(HaveOccurred())
			found := false
			for _, o := range result.Objects {
				if o.Key == objectKey {
					found = true
					break
				}
			}
			Expect(found).To(Equal(expect))
		}

		By("checking bucket before put")
		checkObjectKey(false)

		By("checking put and get")
		ExecSafeAt(boot0, "curl", "-f", "-XPUT", "--data-binary", "@/bin/cat", s3gwObjectURL)
		ExecSafeAt(boot0, "curl", "-f", "-o", "session-log-got.dat", s3gwObjectURL)
		ExecSafeAt(boot0, "cmp", "/bin/cat", "session-log-got.dat")

		By("checking bucket after put")
		checkObjectKey(true)

		By("checking delete")
		ExecSafeAt(boot0, "curl", "-f", "-XDELETE", s3gwObjectURL)

		By("checking bucket after delete")
		checkObjectKey(false)
	})

	It("check access control", func() {
		stdout, stderr, err := ExecAt(boot0, "ckecli", "ssh", "10.69.0.4", "curl -f "+s3gwBucketEndpoint)
		Expect(err).Should(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})
}
