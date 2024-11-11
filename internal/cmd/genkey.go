package cmd

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
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	"github.com/urfave/cli/v2"
)

const (
	skipMicrosoftCertsFlag = "skip-microsoft-certs-I-KNOW-WHAT-IM-DOING"
	customCertDirFlag      = "custom-cert-dir"
)

var GenKeyCmd = cli.Command{
	Name:      "genkey",
	Aliases:   []string{"gk"},
	Usage:     "Generate secureboot keys under the uuid generated by NAME",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "keys/",
			Usage:   "Output directory for the keys",
		},
		&cli.StringFlag{
			Name:    "expiration-in-days",
			Aliases: []string{"e"},
			Value:   "365",
			Usage:   "In how many days from today should the certificates expire",
		},
		&cli.BoolFlag{
			Name:  skipMicrosoftCertsFlag,
			Value: false,
			Usage: "When set to true, microsoft certs are not included in the KEK and db files. THIS COULD BRICK YOUR SYSTEM! (https://wiki.archlinux.org/title/Unified_Extensible_Firmware_Interface/Secure_Boot#Enrolling_Option_ROM_digests). Only use this if you are sure your hardware doesn't need the microsoft certs!",
		},
		&cli.StringFlag{
			Name:  customCertDirFlag,
			Usage: "Path to a directory containing custom certificates to enroll",
		},
	},
	Action: func(ctx *cli.Context) error {
		// TODO: Implement log level
		logger := sdkTypes.NewKairosLogger("auroraboot", "debug", false)

		skipMicrosoftCerts := ctx.Bool(skipMicrosoftCertsFlag)

		name := ctx.Args().Get(0)
		uuid := sbctl.CreateUUID()
		guid := efiutil.StringToGUID(string(uuid))
		output := ctx.String("output")
		if output == "" {
			return errors.New("output not set")
		}
		if err := os.MkdirAll(output, 0700); err != nil {
			return fmt.Errorf("Error creating output directory: %w", err)
		}

		customDerDir := ""
		var err error
		if customCertDir := ctx.String(customCertDirFlag); customCertDir != "" {
			customDerDir, err = prepareCustomDerDir(logger, customCertDir)
			if err != nil {
				return fmt.Errorf("Error preparing custom certs directory: %w", err)
			}
			defer os.RemoveAll(customDerDir)
		}

		for _, keyType := range []string{"PK", "KEK", "db"} {
			logger.Infof("Generating %s", keyType)
			key := filepath.Join(output, fmt.Sprintf("%s.key", keyType))
			pem := filepath.Join(output, fmt.Sprintf("%s.pem", keyType))
			der := filepath.Join(output, fmt.Sprintf("%s.der", keyType))

			args := []string{
				"req", "-nodes", "-x509", "-subj", fmt.Sprintf("/CN=%s-%s/", name, keyType),
				"-keyout", key,
				"-out", pem,
			}
			if expirationInDays := ctx.String("expiration-in-days"); expirationInDays != "" {
				args = append(args, "-days", expirationInDays)
			}
			cmd := exec.Command("openssl", args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("Error generating %s: %w / %s", keyType, err, string(out))
			}
			logger.Infof("%s generated at %s and %s", keyType, key, pem)

			logger.Infof("Converting %s.pem to DER", keyType)
			cmd = exec.Command(
				"openssl", "x509", "-outform", "DER", "-in", pem, "-out", der,
			)
			if out, err = cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("Error generating %s: %w / %s", keyType, err, string(out))
			}
			logger.Infof("%s generated at %s", keyType, der)

			if err = generateAuthKeys(*guid, output, keyType, customDerDir, skipMicrosoftCerts); err != nil {
				return fmt.Errorf("Error generating auth keys: %w", err)
			}

			// Make sure the "der" format also includes the custom certs
			if customDerDir != "" && keyType != "PK" {
				err = appendCustomDerCerts(keyType, customDerDir, output)
				if err != nil {
					return fmt.Errorf("Error appending custom der certs: %w", err)
				}
			}
		}

		// Generate the policy encryption key
		logger.Infof("Generating policy encryption key")
		tpmPrivate := filepath.Join(output, "tpm2-pcr-private.pem")
		cmd := exec.Command(
			"openssl", "genrsa", "-out", tpmPrivate, "2048",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("Error generating tpm2-pcr-private.pem: %w / %s", err, string(out))
		}

		return nil
	},
}

func generateAuthKeys(guid efiutil.EFIGUID, keyPath, keyType, customDerCertDir string, skipMicrosoftCerts bool) error {
	// Prepare all the keys we need
	var err error
	var key []byte

	switch keyType {
	case "PK", "KEK": // PK signs itself and KEK
		key, err = fs.ReadFile(filepath.Join(keyPath, "PK.key"))
	case "db": // KEK signs db
		key, err = fs.ReadFile(filepath.Join(keyPath, "KEK.key"))
	}
	if err != nil {
		return fmt.Errorf("reading the key file %w", err)
	}

	pem, err := fs.ReadFile(filepath.Join(keyPath, keyType+".pem"))
	if err != nil {
		return fmt.Errorf("reading the pem file %w", err)
	}

	sigdb := signature.NewSignatureDatabase()

	if err = sigdb.Append(signature.CERT_X509_GUID, guid, pem); err != nil {
		return fmt.Errorf("appending signature %w", err)
	}

	if keyType != "PK" && !skipMicrosoftCerts {
		// Load microsoft certs
		oemSigDb, err := certs.GetOEMCerts("microsoft", keyType)
		if err != nil {
			return fmt.Errorf("failed to load microsoft keys (type %s): %w", keyType, err)
		}
		sigdb.AppendDatabase(oemSigDb)
	}

	if keyType != "PK" && customDerCertDir != "" {
		customSigDb, err := certs.GetCustomCerts(customDerCertDir, keyType)
		if err != nil {
			return fmt.Errorf("could not load custom keys (type: %s): %w", keyType, err)
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
// from the UEFI firmware and prepares them for use with sbctl.
// The keys are exported in the "authenticated variables" format.
// The keys are expected to be in the "der" format in a specific directory structure.
// The given directory should have the following files:
// - db
// - KEK
// It returns the prepared temporary directory where the keys are stored in
// "der" format in the expected directories.
func prepareCustomDerDir(l sdkTypes.KairosLogger, customCertDir string) (string, error) {
	if customCertDir != "" {
		if _, err := os.Stat(customCertDir); os.IsNotExist(err) {
			return "", fmt.Errorf("custom cert directory does not exist: %s", customCertDir)
		}
	}

	// create a temporary directory to store the custom certs
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
					cert, _ := x509.ParseCertificate(sigEntry.Data)
					if cert != nil {
						keyDir := filepath.Join(tmpDir, "custom", keyType)
						err := os.MkdirAll(keyDir, 0755)
						if err != nil {
							return "", fmt.Errorf("creating directory for key type %s: %w", keyType, err)
						}
						os.WriteFile(filepath.Join(keyDir, fmt.Sprintf("%s%s", keyType, cert.SerialNumber.String())), cert.Raw, 0644)
					}
				default:
					l.Errorf("Not implemented!\n%s\n", sig.SignatureType.Format())
				}
			}
		}
	}

	return tmpDir, nil
}

func appendCustomDerCerts(keyType, customDerCertDir, keyPath string) error {
	if customDerCertDir == "" {
		return nil
	}

	// Open the file to append to
	finalDerFile := filepath.Join(keyPath, keyType+".der")
	final, err := os.OpenFile(finalDerFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening final der file %s: %w", finalDerFile, err)
	}
	defer final.Close()

	customDerDir := filepath.Join(customDerCertDir, "custom", keyType)
	err = filepath.Walk(customDerDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if it's a regular file and if the name starts with the specified prefix
		if !info.IsDir() && strings.HasPrefix(info.Name(), keyType) {
			customData, err := os.ReadFile(filepath.Join(customDerDir, info.Name()))
			if err != nil {
				return fmt.Errorf("reading custom der file %s: %w", keyType, err)
			}

			_, err = final.Write(customData)
			if err != nil {
				return fmt.Errorf("appending custom der file %s: %w", keyType, err)
			}
		}
		return nil
	})

	return err
}