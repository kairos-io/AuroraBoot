package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

// CommandHandler handles command-related REST endpoints.
type CommandHandler struct {
	commands store.CommandStore
	nodes    store.NodeStore
	hub      *ws.Hub
}

// NewCommandHandler creates a new CommandHandler.
func NewCommandHandler(commands store.CommandStore, nodes store.NodeStore, hub *ws.Hub) *CommandHandler {
	return &CommandHandler{
		commands: commands,
		nodes:    nodes,
		hub:      hub,
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
	h.pushCommand(cmd)

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
		h.pushCommand(cmd)
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
		h.pushCommand(cmd)
		created = append(created, cmd)
	}

	return c.JSON(http.StatusCreated, created)
}

// pushCommand attempts to deliver a command via WebSocket if the hub is available
// and the target node is online. Errors are silently ignored since the command
// is already persisted and can be picked up on next connect.
func (h *CommandHandler) pushCommand(cmd *store.NodeCommand) {
	if h.hub == nil {
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
func (h *CommandHandler) UpdateStatus(c echo.Context) error {
	commandID := c.Param("commandID")
	var req updateStatusRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Phase == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "phase is required"})
	}

	if err := h.commands.UpdateStatus(c.Request().Context(), commandID, req.Phase, req.Result); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update command status"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
