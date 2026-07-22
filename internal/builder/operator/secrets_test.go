package operator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

var _ = Describe("materializeSecrets", func() {
	const (
		id = "build-42"
		ns = "kairos-builds"
	)

	It("returns no secrets when neither cloud-config nor Dockerfile is set", func() {
		got := materializeSecrets(id, ns, "", builder.BuildOptions{})
		Expect(got).To(BeEmpty())
	})

	It("emits a single cloud-config secret when only CloudConfig is set", func() {
		got := materializeSecrets(id, ns, "", builder.BuildOptions{
			CloudConfig: "#cloud-config\n",
		})
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("build-42-cloud-config"))
		Expect(got[0].Namespace).To(Equal(ns))
		Expect(got[0].Data).To(HaveKeyWithValue(cloudConfigSecretKey, []byte("#cloud-config\n")))
		Expect(got[0].Labels).To(HaveKeyWithValue(buildIDLabel, id))
	})

	It("emits a single Dockerfile secret when only Dockerfile is set", func() {
		got := materializeSecrets(id, ns, "", builder.BuildOptions{
			Dockerfile: "FROM scratch\n",
		})
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("build-42-dockerfile"))
		Expect(got[0].Namespace).To(Equal(ns))
		Expect(got[0].Data).To(HaveKeyWithValue(dockerfileSecretKey, []byte("FROM scratch\n")))
	})

	It("emits both secrets when both fields are set", func() {
		got := materializeSecrets(id, ns, "", builder.BuildOptions{
			CloudConfig: "#cloud-config\n",
			Dockerfile:  "FROM scratch\n",
		})
		Expect(got).To(HaveLen(2))
		names := []string{got[0].Name, got[1].Name}
		Expect(names).To(ConsistOf("build-42-cloud-config", "build-42-dockerfile"))
	})

	It("emits an upload secret when both uploadURL and UploadToken are set", func() {
		got := materializeSecrets(id, ns, "https://auroraboot.example", builder.BuildOptions{
			UploadToken: "0123456789abcdef",
		})
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("build-42-upload"))
		Expect(got[0].Data).To(HaveKeyWithValue(uploadURLKey, []byte("https://auroraboot.example")))
		Expect(got[0].Data).To(HaveKeyWithValue(uploadTokenKey, []byte("0123456789abcdef")))
	})

	It("skips the upload secret when uploadURL is set but UploadToken is not", func() {
		got := materializeSecrets(id, ns, "https://auroraboot.example", builder.BuildOptions{})
		Expect(got).To(BeEmpty())
	})

	It("skips the upload secret when UploadToken is set but uploadURL is not", func() {
		got := materializeSecrets(id, ns, "", builder.BuildOptions{UploadToken: "0123"})
		Expect(got).To(BeEmpty())
	})
})
