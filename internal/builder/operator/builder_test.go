package operator

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"k8s.io/client-go/rest"
)

var _ = Describe("Operator Builder", func() {
	Describe("New", func() {
		It("returns an error when RESTConfig is nil", func() {
			b, err := New(Config{Namespace: "default"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("RESTConfig"))
			Expect(b).To(BeNil())
		})

		It("returns an error when Namespace is empty", func() {
			b, err := New(Config{RESTConfig: &rest.Config{Host: "https://example.invalid"}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Namespace"))
			Expect(b).To(BeNil())
		})

		It("stores config when inputs are valid", func() {
			b, err := New(Config{
				RESTConfig: &rest.Config{Host: "https://example.invalid"},
				Namespace:  "kairos-builds",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(b).NotTo(BeNil())
			Expect(b.cfg.Namespace).To(Equal("kairos-builds"))
			Expect(b.cfg.RESTConfig.Host).To(Equal("https://example.invalid"))
		})
	})

	Describe("Method stubs", func() {
		var b *Builder

		BeforeEach(func() {
			var err error
			b, err = New(Config{
				RESTConfig: &rest.Config{Host: "https://example.invalid"},
				Namespace:  "default",
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Build returns an error that wraps ErrNotSupported", func() {
			s, err := b.Build(context.Background(), builder.BuildOptions{ID: "x"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, builder.ErrNotSupported)).To(BeTrue())
			Expect(s).To(BeNil())
		})

		It("Status returns an error that wraps ErrNotSupported", func() {
			s, err := b.Status(context.Background(), "x")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, builder.ErrNotSupported)).To(BeTrue())
			Expect(s).To(BeNil())
		})

		It("List returns an error that wraps ErrNotSupported", func() {
			l, err := b.List(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, builder.ErrNotSupported)).To(BeTrue())
			Expect(l).To(BeNil())
		})

		It("Cancel returns an error that wraps ErrNotSupported", func() {
			err := b.Cancel(context.Background(), "x")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, builder.ErrNotSupported)).To(BeTrue())
		})
	})
})
