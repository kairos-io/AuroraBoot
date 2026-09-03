package store_test

import (
	"github.com/kairos-io/AuroraBoot/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("command type constants", func() {
	It("includes extension", func() {
		Expect(store.CmdExtension).To(Equal("extension"))
	})
})
