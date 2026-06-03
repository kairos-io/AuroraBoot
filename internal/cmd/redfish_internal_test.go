package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveServeBindAddr(t *testing.T) {
	tests := []struct {
		name      string
		serveURL  string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{name: "host and port", serveURL: "http://10.0.0.5:8090", want: "10.0.0.5:8090"},
		{name: "https host and port", serveURL: "https://10.0.0.5:8443", want: "10.0.0.5:8443"},
		{name: "no port errors actionably", serveURL: "http://10.0.0.5", wantErr: true, errSubstr: "set --redfish-serve-addr explicitly, e.g. 10.0.0.5:8090"},
		{name: "no host errors", serveURL: "http:///path", wantErr: true, errSubstr: "has no host"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deriveServeBindAddr(tc.serveURL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got addr %q", got)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolveRedfishPassword(t *testing.T) {
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "pw")
	if err := os.WriteFile(pwFile, []byte("from-file\n"), 0600); err != nil {
		t.Fatalf("write pw file: %v", err)
	}
	emptyFile := filepath.Join(dir, "empty")
	if err := os.WriteFile(emptyFile, []byte("\n"), 0600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	tests := []struct {
		name         string
		flag         string
		file         string
		env          string
		stdin        bool
		stdinContent string
		want         string
		wantErr      bool
	}{
		{name: "flag wins over everything", flag: "from-flag", file: pwFile, env: "from-env", stdin: true, stdinContent: "from-stdin", want: "from-flag"},
		{name: "file beats env and stdin", file: pwFile, env: "from-env", stdin: true, stdinContent: "from-stdin", want: "from-file"},
		{name: "env beats stdin", env: "from-env", stdin: true, stdinContent: "from-stdin", want: "from-env"},
		{name: "stdin last resort", stdin: true, stdinContent: "from-stdin\n", want: "from-stdin"},
		{name: "none provided errors", wantErr: true},
		{name: "empty file errors", file: emptyFile, wantErr: true},
		{name: "empty stdin errors", stdin: true, stdinContent: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv(redfishPasswordEnv, tc.env)
			} else {
				// Ensure no ambient value leaks in; t.Setenv restores on cleanup.
				t.Setenv(redfishPasswordEnv, "")
				_ = os.Unsetenv(redfishPasswordEnv)
			}

			got, err := resolveRedfishPassword(tc.flag, tc.file, tc.stdin, strings.NewReader(tc.stdinContent))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got password %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
