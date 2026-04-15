package handlers

import (
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// SettingsHandler handles settings-related REST endpoints.
type SettingsHandler struct {
	mu           sync.RWMutex
	regToken     *string // keep pointer for middleware compatibility
	regTokenFile string  // path to persist the token so rotations survive restarts
}

// NewSettingsHandler creates a new SettingsHandler.
// regToken is a pointer so that rotations are visible to other handlers.
// regTokenFile is the path where the token is persisted; pass "" to disable persistence.
func NewSettingsHandler(regToken *string, regTokenFile string) *SettingsHandler {
	return &SettingsHandler{regToken: regToken, regTokenFile: regTokenFile}
}

// GetRegistrationToken handles GET /api/v1/settings/registration-token.
//
//	@Summary	Read the current registration token
//	@Tags		Settings
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{object}	APIRegistrationTokenResponse
//	@Router		/api/v1/settings/registration-token [get]
func (h *SettingsHandler) GetRegistrationToken(c echo.Context) error {
	h.mu.RLock()
	token := *h.regToken
	h.mu.RUnlock()
	return c.JSON(http.StatusOK, map[string]string{
		"registrationToken": token,
	})
}

// RotateRegistrationToken handles POST /api/v1/settings/registration-token/rotate.
//
//	@Summary		Rotate the registration token
//	@Description	Generates and persists a new token. Already-registered nodes keep working because they authenticate with their api-key.
//	@Tags			Settings
//	@Produce		json
//	@Security		AdminBearer
//	@Success		200	{object}	APIRegistrationTokenResponse
//	@Router			/api/v1/settings/registration-token/rotate [post]
func (h *SettingsHandler) RotateRegistrationToken(c echo.Context) error {
	h.mu.Lock()
	*h.regToken = uuid.New().String()
	token := *h.regToken
	h.mu.Unlock()
	if h.regTokenFile != "" {
		_ = os.WriteFile(h.regTokenFile, []byte(token), 0600)
	}
	return c.JSON(http.StatusOK, map[string]string{
		"registrationToken": token,
	})
}
