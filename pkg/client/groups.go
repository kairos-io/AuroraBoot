package client

import (
	"context"
	"net/http"
)

// GroupsService groups the node-group endpoints.
type GroupsService struct{ c *Client }

// Create creates a new group.
func (s *GroupsService) Create(ctx context.Context, name, description string) (*Group, error) {
	body := map[string]string{"name": name, "description": description}
	var out Group
	if err := s.c.do(ctx, http.MethodPost, "/api/v1/groups", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every group.
func (s *GroupsService) List(ctx context.Context) ([]Group, error) {
	var out []Group
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/groups", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches a single group by ID.
func (s *GroupsService) Get(ctx context.Context, groupID string) (*Group, error) {
	var out Group
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/groups/"+groupID, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update patches a group's name and/or description. Empty fields are ignored server-side.
func (s *GroupsService) Update(ctx context.Context, groupID, name, description string) (*Group, error) {
	body := map[string]string{"name": name, "description": description}
	var out Group
	if err := s.c.do(ctx, http.MethodPut, "/api/v1/groups/"+groupID, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a group. Member nodes are detached (their groupID is
// cleared) in the same transaction; nodes themselves are never deleted.
func (s *GroupsService) Delete(ctx context.Context, groupID string) error {
	return s.c.do(ctx, http.MethodDelete, "/api/v1/groups/"+groupID, nil, nil, nil)
}

// SendCommand queues a command for every member of a group.
func (s *GroupsService) SendCommand(ctx context.Context, groupID string, req CreateCommandRequest) ([]NodeCommand, error) {
	var out []NodeCommand
	if err := s.c.do(ctx, http.MethodPost, "/api/v1/groups/"+groupID+"/commands", nil, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}
