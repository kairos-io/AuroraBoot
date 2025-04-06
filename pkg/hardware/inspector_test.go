package hardware

import (
	"testing"
)

func TestValidateRequirements(t *testing.T) {
	inspector := &Inspector{}

	tests := []struct {
		name    string
		info    *SystemInfo
		reqs    *Requirements
		wantErr bool
		errMsg  string
	}{
		{
			name: "meets requirements",
			info: &SystemInfo{
				MemoryGiB:      8,
				ProcessorCount: 4,
			},
			reqs: &Requirements{
				MinMemoryGiB: 4,
				MinCPUs:      2,
			},
			wantErr: false,
		},
		{
			name: "insufficient memory",
			info: &SystemInfo{
				MemoryGiB:      2,
				ProcessorCount: 4,
			},
			reqs: &Requirements{
				MinMemoryGiB: 4,
				MinCPUs:      2,
			},
			wantErr: true,
			errMsg:  "insufficient memory",
		},
		{
			name: "insufficient CPUs",
			info: &SystemInfo{
				MemoryGiB:      8,
				ProcessorCount: 1,
			},
			reqs: &Requirements{
				MinMemoryGiB: 4,
				MinCPUs:      2,
			},
			wantErr: true,
			errMsg:  "insufficient CPUs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inspector.ValidateRequirements(tt.info, tt.reqs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequirements() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("ValidateRequirements() error message = %v, want %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestHasFeature(t *testing.T) {
	inspector := &Inspector{}

	tests := []struct {
		name    string
		feature string
		want    bool
	}{
		{
			name:    "UEFI support",
			feature: "UEFI",
			want:    true,
		},
		{
			name:    "IPMI support",
			feature: "IPMI",
			want:    true,
		},
		{
			name:    "unsupported feature",
			feature: "unsupported",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inspector.hasFeature(&SystemInfo{}, tt.feature)
			if got != tt.want {
				t.Errorf("hasFeature() = %v, want %v", got, tt.want)
			}
		})
	}
}
