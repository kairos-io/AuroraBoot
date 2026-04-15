package client

import (
	"context"
	"io"
	"net/http"
)

// ArtifactsService groups the image-build endpoints.
type ArtifactsService struct{ c *Client }

// Create kicks off an asynchronous build. The response is the initial
// record with phase Pending; poll Get or subscribe to the UI
// WebSocket to watch it progress.
func (s *ArtifactsService) Create(ctx context.Context, req CreateArtifactRequest) (*Artifact, error) {
	var out Artifact
	if err := s.c.do(ctx, http.MethodPost, "/api/v1/artifacts", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every known artifact build.
func (s *ArtifactsService) List(ctx context.Context) ([]Artifact, error) {
	var out []Artifact
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/artifacts", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches one artifact by ID.
func (s *ArtifactsService) Get(ctx context.Context, id string) (*Artifact, error) {
	var out Artifact
	if err := s.c.do(ctx, http.MethodGet, "/api/v1/artifacts/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Logs returns the full build log snapshot as a plain string. For
// real-time updates subscribe to the UI WebSocket.
func (s *ArtifactsService) Logs(ctx context.Context, id string) (string, error) {
	body, _, err := s.c.doRaw(ctx, http.MethodGet, "/api/v1/artifacts/"+id+"/logs", nil, nil, "")
	if err != nil {
		return "", err
	}
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Cancel aborts a running build.
func (s *ArtifactsService) Cancel(ctx context.Context, id string) error {
	return s.c.do(ctx, http.MethodPost, "/api/v1/artifacts/"+id+"/cancel", nil, nil, nil)
}

// Delete removes an artifact and its output files.
func (s *ArtifactsService) Delete(ctx context.Context, id string) error {
	return s.c.do(ctx, http.MethodDelete, "/api/v1/artifacts/"+id, nil, nil, nil)
}

// ClearFailed deletes every artifact currently in the Error phase.
func (s *ArtifactsService) ClearFailed(ctx context.Context) error {
	return s.c.do(ctx, http.MethodDelete, "/api/v1/artifacts/failed", nil, nil, nil)
}

// Update patches artifact metadata (name and/or saved flag).
func (s *ArtifactsService) Update(ctx context.Context, id string, req UpdateArtifactRequest) (*Artifact, error) {
	var out Artifact
	if err := s.c.do(ctx, http.MethodPatch, "/api/v1/artifacts/"+id, nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Download streams a produced artifact file (an ISO, UKI, raw disk,
// netboot file...). The caller owns the returned ReadCloser and must
// close it.
func (s *ArtifactsService) Download(ctx context.Context, id, filename string) (io.ReadCloser, error) {
	body, _, err := s.c.doRaw(ctx, http.MethodGet, "/api/v1/artifacts/"+id+"/download/"+filename, nil, nil, "")
	return body, err
}
