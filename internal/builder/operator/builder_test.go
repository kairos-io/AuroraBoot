package operator

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	buildv1alpha2 "github.com/kairos-io/kairos-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

// newFakeBuilder builds a Builder wired to controller-runtime's fake client so
// unit tests can drive Create/Get/List/Delete without a real apiserver. It
// also injects a typed-client fake so Build's spawned log-streaming goroutine
// has something to talk to without reaching the network.
func newFakeBuilder(namespace string, objs ...client.Object) (*Builder, client.Client) {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(scheme)).To(Succeed())

	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	cs := k8sfake.NewSimpleClientset()

	b, err := newWithFactory(Config{
		RESTConfig: &rest.Config{Host: "https://fake.invalid"},
		Namespace:  namespace,
	}, func(_ Config, _ *runtime.Scheme) (client.Client, error) {
		return fc, nil
	}, func(_ Config) (kubernetes.Interface, error) {
		return cs, nil
	})
	Expect(err).NotTo(HaveOccurred())
	return b, fc
}

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
	})

	Describe("Build", func() {
		It("creates an OSArtifact and returns Pending status", func() {
			ctx := context.Background()
			b, fc := newFakeBuilder("kairos-builds")

			status, err := b.Build(ctx, builder.BuildOptions{
				ID:        "build-1",
				BaseImage: "quay.io/kairos/ubuntu:v3.6.0",
				Source:    builder.ImageSource{Arch: "amd64"},
				Outputs:   builder.OutputOptions{ISO: true},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ID).To(Equal("build-1"))
			Expect(status.Phase).To(Equal(builder.BuildPending))

			got := &buildv1alpha2.OSArtifact{}
			Expect(fc.Get(ctx, types.NamespacedName{Name: "build-1", Namespace: "kairos-builds"}, got)).To(Succeed())
			Expect(got.Spec.Image.Ref).To(Equal("quay.io/kairos/ubuntu:v3.6.0"))
			Expect(got.Labels).To(HaveKeyWithValue(buildIDLabel, "build-1"))
		})

		It("generates an ID when none is supplied", func() {
			ctx := context.Background()
			b, _ := newFakeBuilder("kairos-builds")

			status, err := b.Build(ctx, builder.BuildOptions{
				BaseImage: "quay.io/kairos/ubuntu:v3.6.0",
				Source:    builder.ImageSource{Arch: "amd64"},
				Outputs:   builder.OutputOptions{ISO: true},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ID).NotTo(BeEmpty())
		})

		It("materializes a cloud-config Secret owned by the CR", func() {
			ctx := context.Background()
			b, fc := newFakeBuilder("kairos-builds")

			_, err := b.Build(ctx, builder.BuildOptions{
				ID:          "build-cc",
				BaseImage:   "quay.io/kairos/ubuntu:v3.6.0",
				Source:      builder.ImageSource{Arch: "amd64"},
				Outputs:     builder.OutputOptions{ISO: true},
				CloudConfig: "#cloud-config\n",
			})
			Expect(err).NotTo(HaveOccurred())

			sec := &corev1.Secret{}
			Expect(fc.Get(ctx, types.NamespacedName{Name: "build-cc-cloud-config", Namespace: "kairos-builds"}, sec)).To(Succeed())
			Expect(sec.Data).To(HaveKeyWithValue("cloud-config", []byte("#cloud-config\n")))
			Expect(sec.OwnerReferences).To(HaveLen(1))
			Expect(sec.OwnerReferences[0].Kind).To(Equal("OSArtifact"))
			Expect(sec.OwnerReferences[0].Name).To(Equal("build-cc"))
		})

		It("returns ErrInvalidBuildOptions on invalid arch", func() {
			ctx := context.Background()
			b, _ := newFakeBuilder("kairos-builds")

			_, err := b.Build(ctx, builder.BuildOptions{
				ID:        "bad",
				BaseImage: "quay.io/kairos/ubuntu:v3.6.0",
				Source:    builder.ImageSource{Arch: "riscv64"},
				Outputs:   builder.OutputOptions{ISO: true},
			})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, builder.ErrInvalidBuildOptions)).To(BeTrue())
		})
	})

	Describe("Status", func() {
		It("translates operator phases into AuroraBoot phases", func() {
			ctx := context.Background()
			base := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "build-s", Namespace: "kairos-builds", Labels: map[string]string{buildIDLabel: "build-s"}},
				Status:     buildv1alpha2.OSArtifactStatus{Phase: buildv1alpha2.Exporting, Message: "packing artifacts"},
			}
			b, _ := newFakeBuilder("kairos-builds", base)

			got, err := b.Status(ctx, "build-s")
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Phase).To(Equal(builder.BuildBuilding))
			Expect(got.Message).To(Equal("packing artifacts"))
		})

		It("defaults an empty phase to Pending", func() {
			ctx := context.Background()
			base := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "build-fresh", Namespace: "kairos-builds"},
			}
			b, _ := newFakeBuilder("kairos-builds", base)

			got, err := b.Status(ctx, "build-fresh")
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Phase).To(Equal(builder.BuildPending))
		})

		It("returns ErrNotFound for an unknown ID", func() {
			ctx := context.Background()
			b, _ := newFakeBuilder("kairos-builds")

			_, err := b.Status(ctx, "does-not-exist")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrNotFound)).To(BeTrue())
		})
	})

	Describe("List", func() {
		It("returns every labelled OSArtifact translated into BuildStatus", func() {
			ctx := context.Background()
			a := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "kairos-builds", Labels: map[string]string{buildIDLabel: "a"}},
				Status:     buildv1alpha2.OSArtifactStatus{Phase: buildv1alpha2.Ready},
			}
			b := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "kairos-builds", Labels: map[string]string{buildIDLabel: "b"}},
				Status:     buildv1alpha2.OSArtifactStatus{Phase: buildv1alpha2.Error, Message: "boom"},
			}
			untagged := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "untagged", Namespace: "kairos-builds"},
			}

			bld, _ := newFakeBuilder("kairos-builds", a, b, untagged)

			got, err := bld.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(2))
			ids := []string{got[0].ID, got[1].ID}
			Expect(ids).To(ConsistOf("a", "b"))
		})
	})

	Describe("Cancel", func() {
		It("deletes the OSArtifact CR", func() {
			ctx := context.Background()
			a := &buildv1alpha2.OSArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "victim", Namespace: "kairos-builds", Labels: map[string]string{buildIDLabel: "victim"}},
			}
			bld, fc := newFakeBuilder("kairos-builds", a)

			Expect(bld.Cancel(ctx, "victim")).To(Succeed())

			got := &buildv1alpha2.OSArtifact{}
			err := fc.Get(ctx, types.NamespacedName{Name: "victim", Namespace: "kairos-builds"}, got)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("is a no-op on an unknown ID", func() {
			ctx := context.Background()
			bld, _ := newFakeBuilder("kairos-builds")
			Expect(bld.Cancel(ctx, "ghost")).To(Succeed())
		})
	})
})
