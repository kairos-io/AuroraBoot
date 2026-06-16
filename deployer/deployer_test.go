package deployer_test

import (
	"github.com/kairos-io/AuroraBoot/deployer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadByte", func() {
	It("reads the allow-insecure-registries key", func() {
		cfg, _, err := deployer.LoadByte([]byte("allow-insecure-registries: true\n"))
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.AllowInsecureRegistries).To(BeTrue())
	})

	It("still honours the deprecated insecure key", func() {
		cfg, _, err := deployer.LoadByte([]byte("insecure: true\n"))
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.AllowInsecureRegistries).To(BeTrue())
	})

	It("defaults both flags to false when neither key is present", func() {
		cfg, _, err := deployer.LoadByte([]byte("arch: amd64\n"))
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.AllowInsecureRegistries).To(BeFalse())
	})
})
