package secureboot

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxboron/go-uefi/efi/signature"
	efiutil "github.com/foxboron/go-uefi/efi/util"
	"github.com/foxboron/sbctl/certs"
	"github.com/kairos-io/kairos/v4/sdk/types/logger"
)

// writeKeyPair generates an RSA key and a self-signed certificate and writes
// them as <name>.key (PKCS8 PEM) and <name>.pem, which is the layout
// generateAuthKeys reads. Done in Go rather than by shelling out so the test
// does not need openssl on PATH.
func writeKeyPair(t *testing.T, dir, name string) *x509.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate %s key: %v", name, err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "auroraboot-test-" + name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create %s cert: %v", name, err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal %s key: %v", name, err)
	}

	writeFile(t, filepath.Join(dir, name+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	writeFile(t, filepath.Join(dir, name+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse %s cert: %v", name, err)
	}
	return cert
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// oemCertDERs returns the DER bytes of the Microsoft certs sbctl bundles for a
// variable. These are the certs GenerateKeySet adds unless SkipMicrosoftCerts.
func oemCertDERs(t *testing.T, variable string) [][]byte {
	t.Helper()
	sigdb, err := certs.GetOEMCerts("microsoft", variable)
	if err != nil {
		t.Fatalf("GetOEMCerts(%s): %v", variable, err)
	}
	var out [][]byte
	for _, sl := range *sigdb {
		for _, s := range sl.Signatures {
			out = append(out, bytes.Clone(s.Data))
		}
	}
	if len(out) == 0 {
		t.Fatalf("sbctl bundles no microsoft certs for %s", variable)
	}
	return out
}

// customDirWith builds the prepared-DER directory layout that
// generateAuthKeys hands to certs.GetCustomCerts: <dir>/custom/<variable>/*.
// This stands in for a user's UEFI export that has been run through
// prepareCustomDerDir.
func customDirWith(t *testing.T, variable string, ders [][]byte) string {
	t.Helper()
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "custom", variable)
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", keyDir, err)
	}
	for i, der := range ders {
		writeFile(t, filepath.Join(keyDir, variable+"-custom-"+big.NewInt(int64(i)).String()), der)
	}
	return dir
}

// countInEsl reports how many entries of the signature database in path carry
// exactly these certificate bytes, across every signature list and regardless
// of the owner GUID recorded for the entry.
func countInEsl(t *testing.T, path string, der []byte) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sigdb, err := signature.ReadSignatureDatabase(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	n := 0
	for _, sl := range sigdb {
		if !efiutil.CmpEFIGUID(sl.SignatureType, signature.CERT_X509_GUID) {
			continue
		}
		for _, s := range sl.Signatures {
			if bytes.Equal(s.Data, der) {
				n++
			}
		}
	}
	return n
}

// TestGenerateAuthKeys_SkipsCertsAlreadyEnrolled covers kairos-io/kairos#2432.
//
// A user who exports KEK/db from their firmware and passes the export through
// --custom-cert-dir, without also passing --skip-microsoft-certs, hands us the
// Microsoft certs twice: once from sbctl's bundled copy and once from their own
// export. The two arrive under different owner GUIDs (certs.GetOEMCerts tags
// them "microsoft", certs.GetCustomCerts tags everything "custom"), so the
// library's own owner+data dedup cannot see it, and SignatureDatabase.
// AppendDatabase does no membership check at all. The cert must still land in
// the enrolled database exactly once.
func TestGenerateAuthKeys_SkipsCertsAlreadyEnrolled(t *testing.T) {
	for _, variable := range []string{"KEK", "db"} {
		t.Run(variable, func(t *testing.T) {
			dir := t.TempDir()
			writeKeyPair(t, dir, "PK")
			signerFor := map[string]string{"KEK": "PK", "db": "KEK"}
			if signerFor[variable] == "KEK" {
				writeKeyPair(t, dir, "KEK")
			}
			own := writeKeyPair(t, dir, variable)

			msCerts := oemCertDERs(t, variable)
			customDir := customDirWith(t, variable, msCerts)

			guid := efiutil.StringToGUID("88a69775-5ad7-45d9-9f34-cec43e1f1989")
			if err := generateAuthKeys(testLogger(), *guid, dir, variable, customDir, false); err != nil {
				t.Fatalf("generateAuthKeys(%s): %v", variable, err)
			}

			esl := filepath.Join(dir, variable+".esl")
			for i, der := range msCerts {
				if got := countInEsl(t, esl, der); got != 1 {
					t.Errorf("%s.esl: microsoft cert %d appears %d times, want 1", variable, i, got)
				}
			}
			if got := countInEsl(t, esl, own.Raw); got != 1 {
				t.Errorf("%s.esl: our own cert appears %d times, want 1", variable, got)
			}
		})
	}
}

// TestGenerateAuthKeys_KeepsDistinctCustomCerts pins the other half: a custom
// cert that is NOT already in the database still has to be enrolled. Without
// this, "skip duplicates" could be satisfied by dropping custom certs wholesale.
func TestGenerateAuthKeys_KeepsDistinctCustomCerts(t *testing.T) {
	dir := t.TempDir()
	writeKeyPair(t, dir, "PK")
	own := writeKeyPair(t, dir, "KEK")

	vendorDir := t.TempDir()
	vendor := writeKeyPair(t, vendorDir, "vendor")
	customDir := customDirWith(t, "KEK", [][]byte{vendor.Raw})

	guid := efiutil.StringToGUID("88a69775-5ad7-45d9-9f34-cec43e1f1989")
	if err := generateAuthKeys(testLogger(), *guid, dir, "KEK", customDir, true); err != nil {
		t.Fatalf("generateAuthKeys: %v", err)
	}

	esl := filepath.Join(dir, "KEK.esl")
	if got := countInEsl(t, esl, vendor.Raw); got != 1 {
		t.Errorf("KEK.esl: distinct custom cert appears %d times, want 1", got)
	}
	if got := countInEsl(t, esl, own.Raw); got != 1 {
		t.Errorf("KEK.esl: our own cert appears %d times, want 1", got)
	}
}

// TestGenerateAuthKeys_NoCustomCertsIsUnchanged guards the common path: with no
// --custom-cert-dir there is nothing to deduplicate, and every bundled
// Microsoft cert must still be enrolled exactly once.
func TestGenerateAuthKeys_NoCustomCertsIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeKeyPair(t, dir, "PK")
	own := writeKeyPair(t, dir, "KEK")

	guid := efiutil.StringToGUID("88a69775-5ad7-45d9-9f34-cec43e1f1989")
	if err := generateAuthKeys(testLogger(), *guid, dir, "KEK", "", false); err != nil {
		t.Fatalf("generateAuthKeys: %v", err)
	}

	esl := filepath.Join(dir, "KEK.esl")
	for i, der := range oemCertDERs(t, "KEK") {
		if got := countInEsl(t, esl, der); got != 1 {
			t.Errorf("KEK.esl: microsoft cert %d appears %d times, want 1", i, got)
		}
	}
	if got := countInEsl(t, esl, own.Raw); got != 1 {
		t.Errorf("KEK.esl: our own cert appears %d times, want 1", got)
	}
}

// testLogger returns a logger that discards output, so the skip messages do
// not drown the test log.
func testLogger() logger.KairosLogger {
	return logger.NewNullLogger()
}

// TestAppendDatabaseSkippingDuplicates_ByteIdenticalWhenNothingIsDropped pins
// the no-regression promise: when the merge finds nothing to skip, it must
// serialize to exactly the bytes SignatureDatabase.AppendDatabase produced
// before. That is what makes this safe for everyone not hitting #2432, and it
// is also what proves the recomputed ListSize is right.
func TestAppendDatabaseSkippingDuplicates_ByteIdenticalWhenNothingIsDropped(t *testing.T) {
	for _, variable := range []string{"KEK", "db"} {
		t.Run(variable, func(t *testing.T) {
			oem, err := certs.GetOEMCerts("microsoft", variable)
			if err != nil {
				t.Fatalf("GetOEMCerts: %v", err)
			}

			want := signature.NewSignatureDatabase()
			want.AppendDatabase(oem)

			got := signature.NewSignatureDatabase()
			appendDatabaseSkippingDuplicates(got, oem, logger.NewNullLogger(), variable, "Microsoft")

			if !bytes.Equal(got.Bytes(), want.Bytes()) {
				t.Errorf("%s: deduplicating merge changed the bytes of a merge with no duplicates\n got %d bytes\nwant %d bytes",
					variable, len(got.Bytes()), len(want.Bytes()))
			}
		})
	}
}
