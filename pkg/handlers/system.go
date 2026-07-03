package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// SystemInfo carries the values the /api/v1/system endpoints expose. Populated
// at wire time in runWeb from the flags plus the resolved kube REST config.
type SystemInfo struct {
	Backend           string // "local" or "operator"
	Cluster           string // REST config Host when Backend=="operator"; empty otherwise
	Namespace         string // OSArtifact namespace when Backend=="operator"; empty otherwise
	DownloadSupported bool
}

// SystemHandler serves the /api/v1/system/* introspection endpoints.
type SystemHandler struct {
	info SystemInfo
}

// NewSystemHandler creates a SystemHandler that reports the given wire-time info.
func NewSystemHandler(info SystemInfo) *SystemHandler {
	return &SystemHandler{info: info}
}

// GetBuilder handles GET /api/v1/system/builder.
//
//	@Summary	Report the active builder backend
//	@Tags		System
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{object}	APISystemBuilder
//	@Router		/api/v1/system/builder [get]
func (h *SystemHandler) GetBuilder(c echo.Context) error {
	return c.JSON(http.StatusOK, APISystemBuilder(h.info))
}
