package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/kairos-io/AuroraBoot/internal/secrets"
	"github.com/kairos-io/AuroraBoot/pkg/hadron"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// SettingHadronRegistryCreds is the settings key backing the persisted registry
// credentials list. The value is a JSON array of settingsCredential entries;
// each entry's Password is stored as an AES-256-GCM ciphertext string (produced
// by internal/secrets.Cipher.Encrypt) rather than plaintext.
const SettingHadronRegistryCreds = "hadron.registryCredentials"

// HadronHandler serves the read-only hadron catalog (base tags, firmware and
// layers) and the encrypted registry-credentials list used by push-mode builds.
// It also exposes an AuthProvider closure that the builder invokes at build
// time to hand credentials to pkg/hadron.Build without any handler entanglement.
type HadronHandler struct {
	catalog  *hadron.Catalog
	settings store.SettingsStore
	cipher   *secrets.Cipher
	mu       sync.Mutex
}

// NewHadronHandler wires a handler with a Catalog, settings store and cipher.
// The catalog may be nil (endpoints return 503); the settings store may be nil
// (credentials endpoints return 503); the cipher may be nil (credentials must
// then be empty on write, and any existing entries decrypt to their raw value,
// matching the store's BMCTarget graceful-migration behavior).
func NewHadronHandler(catalog *hadron.Catalog, settings store.SettingsStore, cipher *secrets.Cipher) *HadronHandler {
	return &HadronHandler{catalog: catalog, settings: settings, cipher: cipher}
}

// GetBaseVersions handles GET /api/v1/hadron/base-versions.
//
//	@Summary	List Hadron base image tags
//	@Tags		Hadron
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{array}	string
//	@Router		/api/v1/hadron/base-versions [get]
func (h *HadronHandler) GetBaseVersions(c echo.Context) error {
	if h.catalog == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "hadron catalog is not configured"})
	}
	tags, err := h.catalog.BaseVersions(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	if tags == nil {
		tags = []string{}
	}
	return c.JSON(http.StatusOK, tags)
}

// GetFirmware handles GET /api/v1/hadron/firmware.
//
//	@Summary	List Hadron firmware images
//	@Tags		Hadron
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{array}	hadron.FirmwareItem
//	@Router		/api/v1/hadron/firmware [get]
func (h *HadronHandler) GetFirmware(c echo.Context) error {
	if h.catalog == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "hadron catalog is not configured"})
	}
	items, err := h.catalog.Firmware(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	if items == nil {
		items = []hadron.FirmwareItem{}
	}
	return c.JSON(http.StatusOK, items)
}

// GetLayers handles GET /api/v1/hadron/layers.
//
//	@Summary	List Hadron software layers
//	@Tags		Hadron
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{array}	hadron.LayerItem
//	@Router		/api/v1/hadron/layers [get]
func (h *HadronHandler) GetLayers(c echo.Context) error {
	if h.catalog == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "hadron catalog is not configured"})
	}
	items, err := h.catalog.Layers(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	if items == nil {
		items = []hadron.LayerItem{}
	}
	return c.JSON(http.StatusOK, items)
}

// registryCredentialAPI is the wire shape for GET/PUT registry credentials.
// The `hasPassword` boolean tells the UI whether a stored entry currently
// carries a password (which the API never returns as plaintext); PUT accepts
// either a Password (rotate) or `keepPassword: true` (preserve the existing
// encrypted value across an unrelated field edit).
type registryCredentialAPI struct {
	Registry     string `json:"registry"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
	KeepPassword bool   `json:"keepPassword,omitempty"`
	HasPassword  bool   `json:"hasPassword,omitempty"`
}

// settingsCredential is the on-disk shape (JSON blob under SettingHadronRegistryCreds).
// Password holds the cipher output, not plaintext.
type settingsCredential struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // encrypted
}

// ListRegistryCredentials handles GET /api/v1/hadron/registry-credentials.
// Passwords are never returned; the UI only learns whether a password is set.
//
//	@Summary	List hadron registry credentials (metadata only)
//	@Tags		Hadron
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{array}	registryCredentialAPI
//	@Router		/api/v1/hadron/registry-credentials [get]
func (h *HadronHandler) ListRegistryCredentials(c echo.Context) error {
	if h.settings == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "settings store is not configured"})
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	stored, err := h.loadCredentialsLocked(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read credentials"})
	}
	out := make([]registryCredentialAPI, 0, len(stored))
	for _, s := range stored {
		out = append(out, registryCredentialAPI{
			Registry:    s.Registry,
			Username:    s.Username,
			HasPassword: s.Password != "",
		})
	}
	return c.JSON(http.StatusOK, out)
}

// PutRegistryCredentials handles PUT /api/v1/hadron/registry-credentials.
// Replaces the persisted list wholesale. Entries with keepPassword=true carry
// over the previous encrypted password when the (registry, username) tuple
// matches an existing row; otherwise Password (plaintext) is encrypted before
// persisting. Empty Password + keepPassword=false clears the password.
//
//	@Summary	Replace hadron registry credentials
//	@Tags		Hadron
//	@Accept		json
//	@Produce	json
//	@Security	AdminBearer
//	@Param		body	body	[]registryCredentialAPI	true	"Credential list"
//	@Success	200
//	@Router		/api/v1/hadron/registry-credentials [put]
func (h *HadronHandler) PutRegistryCredentials(c echo.Context) error {
	if h.settings == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "settings store is not configured"})
	}
	var req []registryCredentialAPI
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	// Basic hygiene: registry+username required per row, no duplicates.
	seen := map[string]bool{}
	for i, r := range req {
		if strings.TrimSpace(r.Registry) == "" || strings.TrimSpace(r.Username) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("entry %d: registry and username are required", i)})
		}
		key := r.Registry + "\x00" + r.Username
		if seen[key] {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("duplicate entry for %s / %s", r.Registry, r.Username)})
		}
		seen[key] = true
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	ctx := c.Request().Context()
	previous, err := h.loadCredentialsLocked(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read existing credentials"})
	}
	prevIndex := map[string]settingsCredential{}
	for _, p := range previous {
		prevIndex[p.Registry+"\x00"+p.Username] = p
	}

	if req == nil {
		req = []registryCredentialAPI{}
	}
	next := make([]settingsCredential, 0, len(req))
	for _, r := range req {
		entry := settingsCredential{Registry: r.Registry, Username: r.Username}
		switch {
		case r.KeepPassword:
			if prev, ok := prevIndex[r.Registry+"\x00"+r.Username]; ok {
				entry.Password = prev.Password
			}
		case r.Password != "":
			if h.cipher == nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "encryption cipher is not configured"})
			}
			enc, err := h.cipher.Encrypt(r.Password)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encrypt password"})
			}
			entry.Password = enc
		}
		next = append(next, entry)
	}

	body, err := json.Marshal(next)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to serialize credentials"})
	}
	if err := h.settings.Set(ctx, SettingHadronRegistryCreds, string(body)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist credentials"})
	}
	return h.listRegistryCredentialsLocked(c, ctx)
}

// listRegistryCredentialsLocked builds the GET response under an already-held
// mutex (called after PUT).
func (h *HadronHandler) listRegistryCredentialsLocked(c echo.Context, ctx context.Context) error {
	stored, err := h.loadCredentialsLocked(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read credentials"})
	}
	out := make([]registryCredentialAPI, 0, len(stored))
	for _, s := range stored {
		out = append(out, registryCredentialAPI{
			Registry:    s.Registry,
			Username:    s.Username,
			HasPassword: s.Password != "",
		})
	}
	return c.JSON(http.StatusOK, out)
}

// loadCredentialsLocked reads and JSON-decodes the persisted list. A missing
// setting is returned as an empty list (not an error) so the first server run
// after upgrade doesn't 500 the UI.
func (h *HadronHandler) loadCredentialsLocked(ctx context.Context) ([]settingsCredential, error) {
	raw, found, err := h.settings.Get(ctx, SettingHadronRegistryCreds)
	if err != nil {
		return nil, err
	}
	if !found || raw == "" {
		return nil, nil
	}
	var out []settingsCredential
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AuthProvider returns a hadron.RegistryAuthProvider closure that reads the
// persisted credential list and decrypts each Password before handing them to
// pkg/hadron.Build. Missing settings / empty list → empty result (nil err), so
// the builder can still push to public registries without auth. A decrypt
// failure fails closed with an error so a corrupted DEK never silently drops a
// credential.
func (h *HadronHandler) AuthProvider() hadron.RegistryAuthProvider {
	return func(ctx context.Context) ([]hadron.RegistryCredential, error) {
		if h.settings == nil {
			return nil, nil
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		stored, err := h.loadCredentialsLocked(ctx)
		if err != nil {
			return nil, err
		}
		if len(stored) == 0 {
			return nil, nil
		}
		out := make([]hadron.RegistryCredential, 0, len(stored))
		for _, s := range stored {
			pw := s.Password
			if pw != "" {
				if h.cipher == nil {
					return nil, errors.New("hadron credentials present but no cipher configured")
				}
				dec, err := h.cipher.Decrypt(pw)
				if err != nil {
					return nil, fmt.Errorf("decrypt credentials for %s: %w", s.Registry, err)
				}
				pw = dec
			}
			out = append(out, hadron.RegistryCredential{
				Registry: s.Registry,
				Username: s.Username,
				Password: pw,
			})
		}
		return out, nil
	}
}
