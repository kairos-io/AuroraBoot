package auroraboot

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sdklogger "github.com/kairos-io/kairos-sdk/types/logger"
)

var _ = Describe("teeKairosLogger", func() {
	It("routes an event emitted through the returned logger to sink", func() {
		var sink bytes.Buffer
		base := sdklogger.NewNullLogger()
		tee := teeKairosLogger(base, &sink)
		tee.Logger.Info().Str("k", "v").Msg("progress line")
		Expect(sink.String()).To(ContainSubstring("progress line"))
		Expect(sink.String()).To(ContainSubstring("k=v"))
	})

	It("does not touch the base logger's own output", func() {
		// A NullLogger discards its own events; teeing must build a fresh
		// zerolog Logger for the returned KairosLogger rather than mutating
		// base. Emitting through base after tee is constructed must land
		// nowhere - in particular, not in sink.
		var sink bytes.Buffer
		base := sdklogger.NewNullLogger()
		_ = teeKairosLogger(base, &sink)
		base.Logger.Info().Msg("via base only")
		Expect(sink.Len()).To(Equal(0))
	})
})
