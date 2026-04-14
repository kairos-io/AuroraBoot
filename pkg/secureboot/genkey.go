// Package secureboot exposes Secure Boot key generation as a reusable Go API.
//
// This package mirrors the behavior of the `auroraboot genkey` CLI command but
// is callable as a library so external tools (for example, daedalus) don't need
// to shell out to the auroraboot binary just to generate a key set.
//
// The generation pipeline still relies on `openssl` being available on PATH for
// X.509 certificate creation; the .auth/.esl files are produced via the
// foxboron/sbctl + foxboron/go-uefi libraries (no external commands required
// for that step).
package secureboot

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/foxboron/go-uefi/efi/signature"
	efiutil "github.com/foxboron/go-uefi/efi/util"
	"github.com/foxboron/go-uefi/efivar"
	"github.com/foxboron/sbctl"
	"github.com/foxboron/sbctl/certs"
	"github.com/foxboron/sbctl/fs"
	"github.com/kairos-io/kairos-sdk/types/logger"
)

// Options configures a Secure Boot key set generation.
type Options struct {
	// Name is the common-name prefix used for the X.509 certificate subjects
	// (e.g. "<name>-PK", "<name>-KEK", "<name>-db"). Required.
	Name string

	// OutputDir is the directory where all generated key files will be written.
	// Created with mode 0700 if it doesn't exist. Required.
	OutputDir string

	// ExpirationInDays controls the X.509 certificate validity. Defaults to "365"
	// when empty.
	ExpirationInDays string

	// SkipMicrosoftCerts disables bundling Microsoft OEM certificates into the
	// KEK and db .auth files. THIS COULD BRICK FIRMWARE — only set to true if
	// you are sure your hardware doesn't need the Microsoft certificates.
	SkipMicrosoftCerts bool

	// CustomCertDir is an optional directory containing custom certificates to
	// enroll alongside the generated keys. The directory is expected to follow
	// the format produced by exporting authenticated EFI variables (see the
	// genkey CLI documentation for details). Empty disables custom cert handling.
	CustomCertDir string

	// Logger is the kairos-sdk logger used for progress messages. If nil, a
	// default debug-level logger is used.
	Logger *logger.KairosLogger
}

// GenerateKeySet generates a complete Secure Boot key set under opts.OutputDir.
// On success the directory will contain:
//
//	PK.key, PK.pem, PK.der, PK.auth, PK.esl
//	KEK.key, KEK.pem, KEK.der, KEK.auth, KEK.esl
//	db.key, db.pem, db.der, db.auth, db.esl
//	tpm2-pcr-private.pem
//
// The .auth files are signed for SecureBoot auto-enrollment (PK signs PK and
// KEK; KEK signs db). When SkipMicrosoftCerts is false (the default), the KEK
// and db databases also include Microsoft's OEM certificates.
func GenerateKeySet(opts Options) error {
	if opts.Name == "" {
		return errors.New("name is required")
	}
	if opts.OutputDir == "" {
		return errors.New("output directory is required")
	}

	log := opts.Logger
	if log == nil {
		l := logger.NewKairosLogger("auroraboot", "debug", false)
		log = &l
	}

	uuid := sbctl.CreateUUID()
	guid := efiutil.StringToGUID(string(uuid))

	if err := os.MkdirAll(opts.OutputDir, 0700); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	customDerDir := ""
	if opts.CustomCertDir != "" {
		var err error
		customDerDir, err = prepareCustomDerDir(*log, opts.CustomCertDir)
		if err != nil {
			return fmt.Errorf("preparing custom certs directory: %w", err)
		}
		defer os.RemoveAll(customDerDir)
	}

	for _, keyType := range []string{"PK", "KEK", "db"} {
		log.Infof("Generating %s", keyType)
		key := filepath.Join(opts.OutputDir, fmt.Sprintf("%s.key", keyType))
		pem := filepath.Join(opts.OutputDir, fmt.Sprintf("%s.pem", keyType))
		der := filepath.Join(opts.OutputDir, fmt.Sprintf("%s.der", keyType))

		args := []string{
			"req", "-nodes", "-x509", "-subj", fmt.Sprintf("/CN=%s-%s/", opts.Name, keyType),
			"-keyout", key,
			"-out", pem,
		}
		expirationDays := opts.ExpirationInDays
		if expirationDays == "" {
			expirationDays = "365"
		}
		args = append(args, "-days", expirationDays)

		cmd := exec.Command("openssl", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("generating %s key+cert: %w / %s", keyType, err, string(out))
		}
		log.Infof("%s generated at %s and %s", keyType, key, pem)

		log.Infof("Converting %s.pem to DER", keyType)
		cmd = exec.Command("openssl", "x509", "-outform", "DER", "-in", pem, "-out", der)
		if out, err = cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("converting %s to DER: %w / %s", keyType, err, string(out))
		}
		log.Infof("%s generated at %s", keyType, der)

		if err := generateAuthKeys(*guid, opts.OutputDir, keyType, customDerDir, opts.SkipMicrosoftCerts); err != nil {
			return fmt.Errorf("generating auth keys for %s: %w", keyType, err)
		}

		if customDerDir != "" && keyType != "PK" {
			if err := appendCustomDerCerts(keyType, customDerDir, opts.OutputDir); err != nil {
				return fmt.Errorf("appending custom der certs for %s: %w", keyType, err)
			}
		}
	}

	// Generate the TPM PCR policy encryption key (used for UKI builds).
	log.Infof("Generating policy encryption key")
	tpmPrivate := filepath.Join(opts.OutputDir, "tpm2-pcr-private.pem")
	cmd := exec.Command("openssl", "genrsa", "-out", tpmPrivate, "2048")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generating tpm2-pcr-private.pem: %w / %s", err, string(out))
	}

	return nil
}

// generateAuthKeys produces the .auth and .esl files for a given key type using
// the foxboron/sbctl and foxboron/go-uefi libraries. PK signs itself and KEK;
// KEK signs db.
func generateAuthKeys(guid efiutil.EFIGUID, keyPath, keyType, customDerCertDir string, skipMicrosoftCerts bool) error {
	var err error
	var key []byte

	switch keyType {
	case "PK", "KEK":
		key, err = fs.ReadFile(filepath.Join(keyPath, "PK.key"))
	case "db":
		key, err = fs.ReadFile(filepath.Join(keyPath, "KEK.key"))
	}
	if err != nil {
		return fmt.Errorf("reading the key file: %w", err)
	}

	pem, err := fs.ReadFile(filepath.Join(keyPath, keyType+".pem"))
	if err != nil {
		return fmt.Errorf("reading the pem file: %w", err)
	}

	sigdb := signature.NewSignatureDatabase()

	if err := sigdb.Append(signature.CERT_X509_GUID, guid, pem); err != nil {
		return fmt.Errorf("appending signature: %w", err)
	}

	if keyType != "PK" && !skipMicrosoftCerts {
		oemSigDb, err := certs.GetOEMCerts("microsoft", keyType)
		if err != nil {
			return fmt.Errorf("loading microsoft keys (type %s): %w", keyType, err)
		}
		sigdb.AppendDatabase(oemSigDb)
	}

	if keyType != "PK" && customDerCertDir != "" {
		customSigDb, err := certs.GetCustomCerts(customDerCertDir, keyType)
		if err != nil {
			return fmt.Errorf("loading custom keys (type %s): %w", keyType, err)
		}
		sigdb.AppendDatabase(customSigDb)
	}

	var efiVarType efivar.Efivar
	switch strings.ToLower(keyType) {
	case "pk":
		efiVarType = efivar.PK
	case "kek":
		efiVarType = efivar.KEK
	case "db":
		efiVarType = efivar.Db
	default:
		return fmt.Errorf("unsupported key type %s", keyType)
	}

	signedDB, err := sbctl.SignDatabase(sigdb, key, pem, efiVarType)
	if err != nil {
		return fmt.Errorf("creating the signed db: %w", err)
	}

	if err := fs.WriteFile(filepath.Join(keyPath, keyType+".auth"), signedDB, 0o644); err != nil {
		return fmt.Errorf("writing the auth file: %w", err)
	}

	if err := fs.WriteFile(filepath.Join(keyPath, keyType+".esl"), sigdb.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing the esl file: %w", err)
	}

	return nil
}

// prepareCustomDerDir takes a cert directory with keys as they are exported
// from the UEFI firmware and prepares them for use with sbctl. The keys are
// expected to be in the "der" format in a specific directory structure under
// "db" and "KEK" subdirectories. It returns the prepared temporary directory.
func prepareCustomDerDir(l logger.KairosLogger, customCertDir string) (string, error) {
	if _, err := os.Stat(customCertDir); os.IsNotExist(err) {
		return "", fmt.Errorf("custom cert directory does not exist: %s", customCertDir)
	}

	tmpDir, err := os.MkdirTemp("", "sbctl-custom-certs-*")
	if err != nil {
		return "", fmt.Errorf("creating temporary directory: %w", err)
	}

	for _, keyType := range []string{"db", "KEK"} {
		b, err := os.ReadFile(filepath.Join(customCertDir, keyType))
		if err != nil {
			return "", fmt.Errorf("reading custom cert file %s: %w", keyType, err)
		}
		f := bytes.NewReader(b)
		siglist, err := signature.ReadSignatureDatabase(f)
		if err != nil {
			return "", fmt.Errorf("reading signature database: %w", err)
		}

		l.Infof("Converting custom certs (type: %s)\n", keyType)
		for _, sig := range siglist {
			for _, sigEntry := range sig.Signatures {
				l.Infof("	Signature Owner: %s\n", sigEntry.Owner.Format())
				switch sig.SignatureType {
				case signature.CERT_X509_GUID, signature.CERT_SHA256_GUID:
					cert, err := x509.ParseCertificate(sigEntry.Data)
					if err != nil {
						l.Errorf("cert error: %s", err)
						continue
					}
					keyDir := filepath.Join(tmpDir, "custom", keyType)
					if err := os.MkdirAll(keyDir, 0755); err != nil {
						return "", fmt.Errorf("creating directory for key type %s: %w", keyType, err)
					}
					if err := os.WriteFile(filepath.Join(keyDir, fmt.Sprintf("%s%s", keyType, cert.SerialNumber.String())), cert.Raw, 0644); err != nil {
						return "", fmt.Errorf("writing custom cert file: %w", err)
					}
				default:
					l.Errorf("Not implemented!\n%s\n", sig.SignatureType.Format())
				}
			}
		}
	}

	return tmpDir, nil
}

// appendCustomDerCerts appends custom DER certificates to the final .der file
// for the given key type.
func appendCustomDerCerts(keyType, customDerCertDir, keyPath string) error {
	if customDerCertDir == "" {
		return nil
	}

	finalDerFile := filepath.Join(keyPath, keyType+".der")
	final, err := os.OpenFile(finalDerFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening final der file %s: %w", finalDerFile, err)
	}
	defer final.Close()

	customDerDir := filepath.Join(customDerCertDir, "custom", keyType)
	return filepath.Walk(customDerDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), keyType) {
			customData, err := os.ReadFile(filepath.Join(customDerDir, info.Name()))
			if err != nil {
				return fmt.Errorf("reading custom der file %s: %w", keyType, err)
			}
			if _, err := final.Write(customData); err != nil {
				return fmt.Errorf("appending custom der file %s: %w", keyType, err)
			}
		}
		return nil
	})
}
