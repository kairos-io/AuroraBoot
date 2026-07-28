package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

// CommandHandler handles command-related REST endpoints.
type CommandHandler struct {
	commands       store.CommandStore
	nodes          store.NodeStore
	hub            *ws.Hub
	nodeExtensions store.NodeExtensionStore
	extensions     store.ExtensionStore
}

// NewCommandHandler creates a new CommandHandler. The nodeExtensions and
// extensions stores are optional; pass nil to opt out of node_extensions
// tracking on the status callback (e.g. in tests that don't exercise it).
func NewCommandHandler(commands store.CommandStore, nodes store.NodeStore, hub *ws.Hub, nodeExtensions store.NodeExtensionStore, extensions store.ExtensionStore) *CommandHandler {
	return &CommandHandler{
		commands:       commands,
		nodes:          nodes,
		hub:            hub,
		nodeExtensions: nodeExtensions,
		extensions:     extensions,
	}
}

// createCommandRequest is the expected body for creating a command.
type createCommandRequest struct {
	Command string            `json:"command"`
	Args    map[string]string `json:"args"`
}

// Create handles POST /api/v1/nodes/:nodeID/commands.
//
//	@Summary	Queue a command for a single node
//	@Tags		Commands
//	@Accept		json
//	@Produce	json
//	@Security	AdminBearer
//	@Param		nodeID	path		string					true	"Node ID"
//	@Param		body	body		APICreateCommandRequest	true	"Command payload"
//	@Success	201		{object}	store.NodeCommand
//	@Router		/api/v1/nodes/{nodeID}/commands [post]
func (h *CommandHandler) Create(c echo.Context) error {
	nodeID := c.Param("nodeID")
	var req createCommandRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Command == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "command is required"})
	}

	cmd := &store.NodeCommand{
		ID:            uuid.New().String(),
		ManagedNodeID: nodeID,
		Command:       req.Command,
		Args:          req.Args,
		Phase:         store.CommandPending,
	}

	if err := h.commands.Create(c.Request().Context(), cmd); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create command"})
	}

	// Push command via WebSocket if node is online.
	h.pushCommand(c.Request().Context(), cmd)

	return c.JSON(http.StatusCreated, cmd)
}

// bulkCommandRequest is the expected body for creating commands in bulk.
type bulkCommandRequest struct {
	Selector store.CommandSelector `json:"selector"`
	Command  string                `json:"command"`
	Args     map[string]string     `json:"args"`
}

// CreateBulk handles POST /api/v1/nodes/commands.
func (h *CommandHandler) CreateBulk(c echo.Context) error {
	var req bulkCommandRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Command == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "command is required"})
	}

	if req.Selector.GroupID == "" && len(req.Selector.NodeIDs) == 0 && len(req.Selector.Labels) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "selector must specify at least one of: groupID, nodeIDs, or labels"})
	}

	ctx := c.Request().Context()
	nodes, err := h.nodes.ListBySelector(ctx, req.Selector)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find nodes"})
	}

	var created []*store.NodeCommand
	for _, node := range nodes {
		cmd := &store.NodeCommand{
			ID:            uuid.New().String(),
			ManagedNodeID: node.ID,
			Command:       req.Command,
			Args:          req.Args,
			Phase:         store.CommandPending,
		}
		if err := h.commands.Create(ctx, cmd); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create command"})
		}
		h.pushCommand(ctx, cmd)
		created = append(created, cmd)
	}

	return c.JSON(http.StatusCreated, created)
}

// CreateForGroup handles POST /api/v1/groups/:id/commands.
func (h *CommandHandler) CreateForGroup(c echo.Context) error {
	groupID := c.Param("id")
	var req createCommandRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Command == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "command is required"})
	}

	ctx := c.Request().Context()
	nodes, err := h.nodes.ListByGroup(ctx, groupID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find nodes in group"})
	}

	var created []*store.NodeCommand
	for _, node := range nodes {
		cmd := &store.NodeCommand{
			ID:            uuid.New().String(),
			ManagedNodeID: node.ID,
			Command:       req.Command,
			Args:          req.Args,
			Phase:         store.CommandPending,
		}
		if err := h.commands.Create(ctx, cmd); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create command"})
		}
		h.pushCommand(ctx, cmd)
		created = append(created, cmd)
	}

	return c.JSON(http.StatusCreated, created)
}

// pushCommand attempts to deliver a command via WebSocket if the hub is available
// and the target node is online.
//
// To avoid double-delivery (push over WS, then the agent re-polls the same
// still-Pending command via GetCommands), the command is atomically claimed
// Pending→Delivered first and only pushed if this call won the claim. When the
// node is offline we do NOT claim, leaving the command Pending for the next poll
// or reconnect. Errors are best-effort: the command is persisted either way.
func (h *CommandHandler) pushCommand(ctx context.Context, cmd *store.NodeCommand) {
	if h.hub == nil || !h.hub.IsOnline(cmd.ManagedNodeID) {
		return
	}
	claimed, err := h.commands.ClaimForDelivery(ctx, cmd.ID)
	if err != nil || !claimed {
		// Either the claim failed or another path (a concurrent poll) already
		// claimed it and will deliver it; don't push a duplicate.
		return
	}
	payload := struct {
		ID      string            `json:"id"`
		Command string            `json:"command"`
		Args    map[string]string `json:"args,omitempty"`
	}{
		ID:      cmd.ID,
		Command: cmd.Command,
		Args:    cmd.Args,
	}
	_ = h.hub.SendCommand(cmd.ManagedNodeID, payload)
}

// Delete handles DELETE /api/v1/nodes/:nodeID/commands/:commandID.
func (h *CommandHandler) Delete(c echo.Context) error {
	commandID := c.Param("commandID")
	ctx := c.Request().Context()
	if err := h.commands.Delete(ctx, commandID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "command not found"})
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearHistory handles DELETE /api/v1/nodes/:nodeID/commands.
func (h *CommandHandler) ClearHistory(c echo.Context) error {
	nodeID := c.Param("nodeID")
	ctx := c.Request().Context()
	if err := h.commands.DeleteTerminal(ctx, nodeID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to clear history"})
	}
	return c.NoContent(http.StatusNoContent)
}

// updateStatusRequest is the expected body for updating command status.
type updateStatusRequest struct {
	Phase  string `json:"phase"`
	Result string `json:"result"`
}

// UpdateStatus handles PUT /api/v1/nodes/:nodeID/commands/:commandID/status.
//
// This handler is mounted on both the agent group (node API key) and the admin
// group (admin bearer). For agent callers we scope the update to the
// authenticated node: a node may only update the status of commands addressed
// to it, never another node's (BOLA). Admins legitimately act across nodes, so
// their updates are unscoped.
func (h *CommandHandler) UpdateStatus(c echo.Context) error {
	commandID := c.Param("commandID")
	nodeID := c.Param("nodeID")
	var req updateStatusRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Phase == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "phase is required"})
	}

	ctx := c.Request().Context()

	// Agent request: node API key auth set the node identity. Scope the update
	// to that node so a foreign or non-existent command surfaces as 403 rather
	// than a silent success (gorm Updates returns nil on zero matched rows).
	if authNodeID := auth.AuthNodeID(c); authNodeID != "" {
		err := h.commands.UpdateStatusForNode(ctx, commandID, authNodeID, req.Phase, req.Result)
		if errors.Is(err, store.ErrCommandNotFound) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
		}
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update command status"})
		}
		// node_extensions tracking on the agent path: same hook the admin path
		// (below) and the WS command-status branch invoke when the phase is
		// terminal.
		if req.Phase == store.CommandCompleted && h.nodeExtensions != nil {
			if cmd, err := h.commands.GetByID(ctx, commandID); err == nil && cmd != nil {
				h.applyExtensionTracking(ctx, authNodeID, cmd)
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	// Admin request: act across nodes.
	if err := h.commands.UpdateStatus(ctx, commandID, req.Phase, req.Result); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update command status"})
	}

	// Track node_extensions only on successful completion.
	if req.Phase == store.CommandCompleted && h.nodeExtensions != nil {
		if cmd, err := h.commands.GetByID(ctx, commandID); err == nil {
			h.applyExtensionTracking(ctx, nodeID, cmd)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// bundledExtension matches the wire shape AuroraBoot emits in
// commands.Args["extensions"]. The optional Version field is server-side
// only — the agent ignores unknown JSON fields.
type bundledExtension struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version,omitempty"`
}

// ApplyExtensionTracking is the public entry point so callers outside this
// package (notably the WebSocket command-status handler at
// pkg/ws/handler.go) can drive node_extensions writes on success. Mirrors
// the body of applyExtensionTracking.
func (h *CommandHandler) ApplyExtensionTracking(ctx context.Context, nodeID string, cmd *store.NodeCommand) {
	h.applyExtensionTracking(ctx, nodeID, cmd)
}

func (h *CommandHandler) applyExtensionTracking(ctx context.Context, nodeID string, cmd *store.NodeCommand) {
	switch cmd.Command {
	case store.CmdExtension:
		h.applyManualExtension(ctx, nodeID, cmd)
	case store.CmdUpgrade, store.CmdUpgradeRecovery:
		h.applyBundledExtensions(ctx, nodeID, cmd)
	}
}

func (h *CommandHandler) applyManualExtension(ctx context.Context, nodeID string, cmd *store.NodeCommand) {
	args := cmd.Args
	action, extType, name, bootState := args["action"], args["type"], args["name"], args["bootState"]
	if name == "" || extType == "" {
		return
	}
	switch action {
	case "install", "enable":
		version := ""
		extensionID := ""
		if h.extensions != nil {
			if ext, err := h.extensions.FindLatestReadyByName(ctx, extType, name); err == nil && ext != nil {
				version = ext.Version
				extensionID = ext.ID
			}
		}
		_ = h.nodeExtensions.Upsert(ctx, &store.NodeExtensionRow{
			NodeID: nodeID, Name: name, Type: extType, BootState: bootState,
			Version: version, ExtensionID: extensionID,
		})
	case "disable":
		_ = h.nodeExtensions.DeleteByScope(ctx, nodeID, extType, name, bootState)
	case "remove":
		_ = h.nodeExtensions.DeleteByName(ctx, nodeID, extType, name)
	}
}

func (h *CommandHandler) applyBundledExtensions(ctx context.Context, nodeID string, cmd *store.NodeCommand) {
	raw := cmd.Args["extensions"]
	if raw == "" {
		return
	}
	var list []bundledExtension
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return
	}
	scope := "active"
	if cmd.Command == store.CmdUpgradeRecovery {
		scope = "recovery"
	}
	for _, e := range list {
		_ = h.nodeExtensions.Upsert(ctx, &store.NodeExtensionRow{
			NodeID: nodeID, Name: e.Name, Type: e.Type, BootState: scope,
			Version: e.Version,
		})
	}
}
