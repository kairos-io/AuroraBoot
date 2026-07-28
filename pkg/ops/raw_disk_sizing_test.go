package ops

import "testing"

func TestSystemImageSize(t *testing.T) {
	tests := []struct {
		name       string
		sourceSize uint64
		configured int64
		want       uint
	}{
		{name: "automatic adds headroom", sourceSize: 7_774, want: 7_974},
		{name: "configured override", sourceSize: 7_774, configured: 12_000, want: 12_000},
		{name: "negative override uses automatic sizing", sourceSize: 7_774, configured: -1, want: 7_974},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := systemImageSize(tt.sourceSize, tt.configured); got != tt.want {
				t.Fatalf("systemImageSize(%d, %d) = %d, want %d", tt.sourceSize, tt.configured, got, tt.want)
			}
		})
	}
}

func TestRecoveryPartitionSizeTracksSystemImage(t *testing.T) {
	const systemSize = uint(12_000)
	const want = uint(24_150)

	if got := recoveryPartitionSize(systemSize); got != want {
		t.Fatalf("recoveryPartitionSize(%d) = %d, want %d", systemSize, got, want)
	}
}
