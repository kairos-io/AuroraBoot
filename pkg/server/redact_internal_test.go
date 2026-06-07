package server

import (
	"strings"
	"testing"
)

// TestRedactToken asserts redactToken never emits the ?token= credential while
// preserving the path and any other query parameters. The access logger runs
// the request URI through this helper, so a leak here means credentials land in
// the access log.
func TestRedactToken(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantNoSec string // secret that must NOT appear in the output
		want      string // exact expected output ("" = don't assert exact)
	}{
		{
			name:      "token only",
			in:        "/api/v1/ws?token=supersecret",
			wantNoSec: "supersecret",
			want:      "/api/v1/ws?token=REDACTED",
		},
		{
			name:      "token with other params preserved",
			in:        "/api/v1/artifacts/5/download/foo?token=secret&x=1",
			wantNoSec: "secret",
		},
		{
			name:      "no token left untouched",
			in:        "/api/v1/nodes?limit=10",
			wantNoSec: "",
			want:      "/api/v1/nodes?limit=10",
		},
		{
			name:      "no query string",
			in:        "/healthz",
			wantNoSec: "",
			want:      "/healthz",
		},
		{
			name:      "unparseable query still hides token",
			in:        "/api/v1/ws?token=secret;%zz",
			wantNoSec: "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactToken(tt.in)

			if tt.wantNoSec != "" && strings.Contains(got, tt.wantNoSec) {
				t.Fatalf("redactToken(%q) = %q, leaked secret %q", tt.in, got, tt.wantNoSec)
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("redactToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// A token param must always become REDACTED.
			if strings.Contains(tt.in, "token=") && !strings.Contains(got, "REDACTED") {
				t.Errorf("redactToken(%q) = %q, expected REDACTED marker", tt.in, got)
			}
		})
	}
}
