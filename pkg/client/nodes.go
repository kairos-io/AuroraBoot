package client

import (
	"context"
	"net/http"
	"net/url"
)

// NodesService groups the node-related endpoints.
type NodesService struct{ c *Client }

// Register registers a new node or re-enrolls one with the same
// machineID. Authentication is the registrationToken carried inside
// the request body — the client's admin/node auth (if any) is
// ignored for this call since the target is a bootstrap endpoint.
//
// Register intentionally does NOT set the client's own api key after
// a successful call: callers that want a fresh client authenticated
// as the new node should use WithNodeAPIKey on the returned response:
//
//	reg, _ := cli.Nodes.Register(ctx, ...)
//	agentCli := cli.WithNodeAPIKey(reg.APIKey)
func (s *NodesService) Register(ctx context.Context, req NodeRegisterRequest) (*NodeRegisterResponse, error) {
	var out NodeRegisterResponse
	if err := s.c.do(ctx, http.MethodPost, "/api/v1/nodes/register", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every registered node, optionally filtered by group
// or a single label pair. Pass nil for no filters.
func (s *NodesService) List(ctx context.Context, opts *NodeListOptions) ([]Node, error) {
	q := url.Values{}
	if opts != nil {
		if opts.GroupID != "" {
			q.Set("group", opts.GroupID)
		}
		if opts.Label != "" {
			q.Set("label", opts.Label)
		}
	}
	var out []Node
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/nodes", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches a single node by ID.
func (s *NodesService) Get(ctx context.Context, nodeID string) (*Node, error) {
	var out Node
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/nodes/"+nodeID, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a node.
func (s *NodesService) Delete(ctx context.Context, nodeID string) error {
	return s.c.do(ctx, http.MethodDelete, "/api/v1/nodes/"+nodeID, nil, nil, nil)
}

// SetLabels replaces a node's labels wholesale.
func (s *NodesService) SetLabels(ctx context.Context, nodeID string, labels map[string]string) error {
	body := map[string]map[string]string{"labels": labels}
	return s.c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/labels", nil, body, nil)
}

// SetGroup assigns a node to a group (or clears it, if groupID is "").
func (s *NodesService) SetGroup(ctx context.Context, nodeID, groupID string) error {
	body := map[string]string{"groupID": groupID}
	return s.c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/group", nil, body, nil)
}

// Heartbeat reports a periodic liveness update from a registered
// node. Transitions the node to Online server-side.
func (s *NodesService) Heartbeat(ctx context.Context, nodeID string, req NodeHeartbeatRequest) error {
	return s.c.do(ctx, http.MethodPost, "/api/v1/nodes/"+nodeID+"/heartbeat", nil, req, nil)
}

// GetCommands fetches the commands queued for a node. Agents see
// only Pending commands to claim; admins see everything.
func (s *NodesService) GetCommands(ctx context.Context, nodeID string) ([]NodeCommand, error) {
	var out []NodeCommand
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/nodes/"+nodeID+"/commands", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
