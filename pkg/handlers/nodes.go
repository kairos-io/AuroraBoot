package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// NodeHandler handles node-related REST endpoints.
type NodeHandler struct {
	nodes       store.NodeStore
	commands    store.CommandStore
	groups      store.GroupStore
	regToken    string
	aurorabootURL string
}

// NewNodeHandler creates a new NodeHandler.
func NewNodeHandler(nodes store.NodeStore, commands store.CommandStore, groups store.GroupStore, regToken string, aurorabootURL string) *NodeHandler {
	return &NodeHandler{
		nodes:       nodes,
		commands:    commands,
		groups:      groups,
		regToken:    regToken,
		aurorabootURL: aurorabootURL,
	}
}

// registerRequest is the expected body for node registration.
type registerRequest struct {
	RegistrationToken string `json:"registrationToken"`
	MachineID         string `json:"machineID"`
	Hostname          string `json:"hostname"`
	AgentVersion      string `json:"agentVersion"`
}

// Register handles POST /api/v1/nodes/register.
//
//	@Summary		Register a node
//	@Description	Idempotent by machineID: if a node with the same machineID already exists, the existing record is returned so the agent can resume with its persisted API key. Authenticated by the registrationToken inside the request body.
//	@Tags			Agent bootstrap
//	@Accept			json
//	@Produce		json
//	@Param			body	body		APIRegisterRequest	true	"Registration payload"
//	@Success		200		{object}	APIRegisterResponse
//	@Success		201		{object}	APIRegisterResponse
//	@Failure		400		{object}	APIError
//	@Failure		401		{object}	APIError
//	@Router			/api/v1/nodes/register [post]
func (h *NodeHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.MachineID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "machineID is required"})
	}

	// Check if node already exists by machineID
	existing, _ := h.nodes.GetByMachineID(c.Request().Context(), req.MachineID)
	if existing != nil {
		// Return existing node info
		return c.JSON(http.StatusOK, map[string]any{
			"id":     existing.ID,
			"apiKey": existing.APIKey,
		})
	}

	node := &store.ManagedNode{
		ID:           uuid.New().String(),
		MachineID:    req.MachineID,
		Hostname:     req.Hostname,
		AgentVersion: req.AgentVersion,
		Phase:        store.PhaseRegistered,
		APIKey:       uuid.New().String(),
		Labels:       make(map[string]string),
	}

	if err := h.nodes.Register(c.Request().Context(), node); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to register node"})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"id":     node.ID,
		"apiKey": node.APIKey,
	})
}

// List handles GET /api/v1/nodes.
//
//	@Summary		List nodes
//	@Description	Returns every registered node. Optional group/label filters.
//	@Tags			Nodes
//	@Produce		json
//	@Security		AdminBearer
//	@Param			group	query		string					false	"Filter by group ID"
//	@Param			label	query		string					false	"Filter by a single key:value label pair"
//	@Success		200		{array}		store.ManagedNode
//	@Failure		401		{object}	APIError
//	@Router			/api/v1/nodes [get]
func (h *NodeHandler) List(c echo.Context) error {
	ctx := c.Request().Context()

	groupParam := c.QueryParam("group")
	labelParam := c.QueryParam("label")

	if groupParam != "" {
		nodes, err := h.nodes.ListByGroup(ctx, groupParam)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list nodes"})
		}
		return c.JSON(http.StatusOK, nodes)
	}

	if labelParam != "" {
		key, value, ok := strings.Cut(labelParam, ":")
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "label must be in key:value format"})
		}
		labels := map[string]string{key: value}
		nodes, err := h.nodes.ListByLabels(ctx, labels)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list nodes"})
		}
		return c.JSON(http.StatusOK, nodes)
	}

	nodes, err := h.nodes.List(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list nodes"})
	}
	return c.JSON(http.StatusOK, nodes)
}

// Get handles GET /api/v1/nodes/:nodeID.
//
//	@Summary		Get a node
//	@Tags			Nodes
//	@Produce		json
//	@Security		AdminBearer
//	@Param			nodeID	path		string	true	"Node ID"
//	@Success		200		{object}	store.ManagedNode
//	@Failure		404		{object}	APIError
//	@Router			/api/v1/nodes/{nodeID} [get]
func (h *NodeHandler) Get(c echo.Context) error {
	nodeID := c.Param("nodeID")
	node, err := h.nodes.GetByID(c.Request().Context(), nodeID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "node not found"})
	}
	return c.JSON(http.StatusOK, node)
}

// Delete handles DELETE /api/v1/nodes/:nodeID.
//
//	@Summary		Delete a node
//	@Tags			Nodes
//	@Security		AdminBearer
//	@Param			nodeID	path	string	true	"Node ID"
//	@Success		204
//	@Failure		404	{object}	APIError
//	@Router			/api/v1/nodes/{nodeID} [delete]
func (h *NodeHandler) Delete(c echo.Context) error {
	nodeID := c.Param("nodeID")
	if err := h.nodes.Delete(c.Request().Context(), nodeID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete node"})
	}
	return c.NoContent(http.StatusNoContent)
}

// setLabelsRequest is the expected body for setting labels.
type setLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

// SetLabels handles PUT /api/v1/nodes/:nodeID/labels.
//
//	@Summary		Replace a node's labels
//	@Tags			Nodes
//	@Accept			json
//	@Produce		json
//	@Security		AdminBearer
//	@Param			nodeID	path		string				true	"Node ID"
//	@Param			body	body		APISetLabelsRequest	true	"Labels payload"
//	@Success		200
//	@Router			/api/v1/nodes/{nodeID}/labels [put]
func (h *NodeHandler) SetLabels(c echo.Context) error {
	nodeID := c.Param("nodeID")
	var req setLabelsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.nodes.SetLabels(c.Request().Context(), nodeID, req.Labels); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to set labels"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// setGroupRequest is the expected body for setting a group.
type setGroupRequest struct {
	GroupID string `json:"groupID"`
}

// SetGroup handles PUT /api/v1/nodes/:nodeID/group.
//
//	@Summary		Assign a node to a group
//	@Tags			Nodes
//	@Accept			json
//	@Security		AdminBearer
//	@Param			nodeID	path		string				true	"Node ID"
//	@Param			body	body		APISetGroupRequest	true	"Target group"
//	@Success		200
//	@Router			/api/v1/nodes/{nodeID}/group [put]
func (h *NodeHandler) SetGroup(c echo.Context) error {
	nodeID := c.Param("nodeID")
	var req setGroupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.nodes.SetGroup(c.Request().Context(), nodeID, req.GroupID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to set group"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// heartbeatRequest is the expected body for a heartbeat.
type heartbeatRequest struct {
	AgentVersion string            `json:"agentVersion"`
	OSRelease    map[string]string `json:"osRelease"`
}

// Heartbeat handles POST /api/v1/nodes/:nodeID/heartbeat.
//
//	@Summary		Agent heartbeat
//	@Description	Transitions the node to Online and records the latest agent version and OS info.
//	@Tags			Agent
//	@Accept			json
//	@Security		NodeAPIKey
//	@Param			nodeID	path		string					true	"Node ID"
//	@Param			body	body		APIHeartbeatRequest	true	"Heartbeat payload"
//	@Success		200
//	@Router			/api/v1/nodes/{nodeID}/heartbeat [post]
func (h *NodeHandler) Heartbeat(c echo.Context) error {
	nodeID := c.Param("nodeID")
	var req heartbeatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.nodes.UpdateHeartbeat(c.Request().Context(), nodeID, req.AgentVersion, req.OSRelease); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update heartbeat"})
	}
	if err := h.nodes.UpdatePhase(c.Request().Context(), nodeID, store.PhaseOnline); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update phase"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetCommands handles GET /api/v1/nodes/:nodeID/commands.
// For agent requests, returns only pending commands and marks them as delivered.
func (h *NodeHandler) GetCommands(c echo.Context) error {
	nodeID := c.Param("nodeID")

	// Check if this is an agent request (node API key auth sets nodeID in context)
	ctxNodeID, _ := c.Get(auth.ContextKeyNodeID).(string)
	isAgent := ctxNodeID != ""

	if isAgent {
		cmds, err := h.commands.GetPending(c.Request().Context(), nodeID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get commands"})
		}
		if cmds == nil {
			cmds = []*store.NodeCommand{}
		}

		// Mark them as delivered
		ids := make([]string, len(cmds))
		for i, cmd := range cmds {
			ids[i] = cmd.ID
		}
		if len(ids) > 0 {
			now := time.Now()
			_ = h.commands.MarkDelivered(c.Request().Context(), ids)
			for _, cmd := range cmds {
				cmd.Phase = store.CommandDelivered
				cmd.DeliveredAt = &now
			}
		}

		return c.JSON(http.StatusOK, cmds)
	}

	// Admin request: return all commands for the node
	cmds, err := h.commands.ListByNode(c.Request().Context(), nodeID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get commands"})
	}
	if cmds == nil {
		cmds = []*store.NodeCommand{}
	}
	return c.JSON(http.StatusOK, cmds)
}

// InstallScript handles GET /api/v1/install-agent.
func (h *NodeHandler) InstallScript(c echo.Context) error {
	script := fmt.Sprintf(`#!/bin/bash
# AuroraBoot agent install script
# Usage: curl -sSL %s/api/v1/install-agent | AURORABOOT_GROUP=mygroup bash

set -e

AURORABOOT_URL="${AURORABOOT_URL:-%s}"
REG_TOKEN="${REGISTRATION_TOKEN:-}"

if [ -z "$AURORABOOT_URL" ]; then
    echo "Error: AURORABOOT_URL is required"
    exit 1
fi
if [ -z "$REG_TOKEN" ]; then
    echo "Error: REGISTRATION_TOKEN is required"
    exit 1
fi
GROUP="${AURORABOOT_GROUP:-}"

echo "Installing AuroraBoot agent..."
echo "Server: ${AURORABOOT_URL}"

# Write phonehome config to /oem/ (Kairos standard config location)
mkdir -p /oem
cat > /oem/phonehome.yaml << EOF
#cloud-config
phonehome:
  url: "${AURORABOOT_URL}"
  registration_token: "${REG_TOKEN}"
  group: "${GROUP}"
EOF

echo "Config written to /oem/phonehome.yaml"

# Start kairos-agent which auto-detects the auroraboot config in /oem and
# installs + starts the kairos-agent-phonehome systemd service.
if ! command -v kairos-agent >/dev/null 2>&1; then
  echo "Error: kairos-agent not found. Install it first — phonehome requires kairos-agent to run." >&2
  exit 1
fi
echo "Starting kairos-agent..."
kairos-agent start
echo "kairos-agent-phonehome service installed and started."

echo "AuroraBoot agent installation complete."
`, h.aurorabootURL, h.aurorabootURL)

	return c.String(http.StatusOK, script)
}
