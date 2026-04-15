package integration_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Install Script", func() {
	It("should return a valid shell script with token and URL", func() {
		resp, err := http.Get(testServerURL + "/api/v1/install-agent")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body := readBody(resp)
		Expect(body).To(ContainSubstring("#!/bin/bash"))
		Expect(body).To(ContainSubstring("AURORABOOT_URL"))
		Expect(body).To(ContainSubstring("/oem/phonehome.yaml"))
		Expect(body).To(ContainSubstring("kairos-agent-phonehome"))
	})
})
