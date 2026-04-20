package handlers_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// fakeNodeStore implements store.NodeStore for testing.
type fakeNodeStore struct {
	mu    sync.Mutex
	nodes []*store.ManagedNode
}

func (f *fakeNodeStore) Register(_ context.Context, n *store.ManagedNode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes = append(f.nodes, n)
	return nil
}

func (f *fakeNodeStore) GetByID(_ context.Context, id string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeNodeStore) GetByMachineID(_ context.Context, mid string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.MachineID == mid {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeNodeStore) GetByAPIKey(_ context.Context, key string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.APIKey == key {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeNodeStore) List(_ context.Context) ([]*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nodes, nil
}

func (f *fakeNodeStore) ListByGroup(_ context.Context, groupID string) ([]*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.ManagedNode
	for _, n := range f.nodes {
		if n.GroupID == groupID {
			result = append(result, n)
		}
	}
	return result, nil
}

func (f *fakeNodeStore) ListByLabels(_ context.Context, labels map[string]string) ([]*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.ManagedNode
	for _, n := range f.nodes {
		match := true
		for k, v := range labels {
			if n.Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, n)
		}
	}
	return result, nil
}

func (f *fakeNodeStore) ListBySelector(_ context.Context, sel store.CommandSelector) ([]*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(sel.NodeIDs) > 0 {
		var result []*store.ManagedNode
		for _, n := range f.nodes {
			for _, id := range sel.NodeIDs {
				if n.ID == id {
					result = append(result, n)
				}
			}
		}
		return result, nil
	}
	if sel.GroupID != "" {
		var result []*store.ManagedNode
		for _, n := range f.nodes {
			if n.GroupID == sel.GroupID {
				result = append(result, n)
			}
		}
		return result, nil
	}
	return f.nodes, nil
}

func (f *fakeNodeStore) UpdateHeartbeat(_ context.Context, id string, agentVersion string, osRelease map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == id {
			n.AgentVersion = agentVersion
			n.OSRelease = osRelease
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeNodeStore) UpdatePhase(_ context.Context, id string, phase string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == id {
			n.Phase = phase
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeNodeStore) SetGroup(_ context.Context, nodeID string, groupID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == nodeID {
			n.GroupID = groupID
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeNodeStore) SetLabels(_ context.Context, nodeID string, labels map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == nodeID {
			n.Labels = labels
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeNodeStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, n := range f.nodes {
		if n.ID == id {
			f.nodes = append(f.nodes[:i], f.nodes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

// fakeCommandStore implements store.CommandStore for testing.
type fakeCommandStore struct {
	mu   sync.Mutex
	cmds []*store.NodeCommand
}

func (f *fakeCommandStore) Create(_ context.Context, cmd *store.NodeCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, cmd)
	return nil
}

func (f *fakeCommandStore) GetByID(_ context.Context, id string) (*store.NodeCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cmd := range f.cmds {
		if cmd.ID == id {
			return cmd, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeCommandStore) GetPending(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.NodeCommand
	for _, cmd := range f.cmds {
		if cmd.ManagedNodeID == nodeID && cmd.Phase == store.CommandPending {
			result = append(result, cmd)
		}
	}
	return result, nil
}

func (f *fakeCommandStore) MarkDelivered(_ context.Context, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cmd := range f.cmds {
		for _, id := range ids {
			if cmd.ID == id {
				cmd.Phase = store.CommandDelivered
			}
		}
	}
	return nil
}

func (f *fakeCommandStore) UpdateStatus(_ context.Context, id string, phase string, result string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cmd := range f.cmds {
		if cmd.ID == id {
			cmd.Phase = phase
			cmd.Result = result
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeCommandStore) ListByNode(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.NodeCommand
	for _, cmd := range f.cmds {
		if cmd.ManagedNodeID == nodeID {
			result = append(result, cmd)
		}
	}
	return result, nil
}

func (f *fakeCommandStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, cmd := range f.cmds {
		if cmd.ID == id {
			f.cmds = append(f.cmds[:i], f.cmds[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeCommandStore) DeleteTerminal(_ context.Context, nodeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var remaining []*store.NodeCommand
	for _, cmd := range f.cmds {
		if cmd.ManagedNodeID == nodeID && (cmd.Phase == store.CommandCompleted || cmd.Phase == store.CommandFailed) {
			continue
		}
		remaining = append(remaining, cmd)
	}
	f.cmds = remaining
	return nil
}

// fakeArtifactStore implements store.ArtifactStore for testing.
type fakeArtifactStore struct {
	mu      sync.Mutex
	records []*store.ArtifactRecord
}

func (f *fakeArtifactStore) Create(_ context.Context, rec *store.ArtifactRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rec)
	return nil
}

func (f *fakeArtifactStore) GetByID(_ context.Context, id string) (*store.ArtifactRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.records {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeArtifactStore) List(_ context.Context) ([]*store.ArtifactRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records, nil
}

func (f *fakeArtifactStore) Update(_ context.Context, rec *store.ArtifactRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.records {
		if r.ID == rec.ID {
			f.records[i] = rec
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeArtifactStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.records {
		if r.ID == id {
			f.records = append(f.records[:i], f.records[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeArtifactStore) DeleteByPhase(_ context.Context, phase string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var remaining []*store.ArtifactRecord
	for _, r := range f.records {
		if r.Phase != phase {
			remaining = append(remaining, r)
		}
	}
	f.records = remaining
	return nil
}

func (f *fakeArtifactStore) GetLogs(_ context.Context, id string) (string, error) {
	return "", nil
}

func (f *fakeArtifactStore) AppendLog(_ context.Context, id string, text string) error {
	return nil
}

// fakeGroupStore implements store.GroupStore for testing.
type fakeGroupStore struct {
	mu     sync.Mutex
	groups []*store.NodeGroup
}

func (f *fakeGroupStore) Create(_ context.Context, g *store.NodeGroup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups = append(f.groups, g)
	return nil
}

func (f *fakeGroupStore) GetByID(_ context.Context, id string) (*store.NodeGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeGroupStore) GetByName(_ context.Context, name string) (*store.NodeGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.groups {
		if g.Name == name {
			return g, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeGroupStore) List(_ context.Context) ([]*store.NodeGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groups, nil
}

func (f *fakeGroupStore) Update(_ context.Context, group *store.NodeGroup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, g := range f.groups {
		if g.ID == group.ID {
			f.groups[i] = group
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeGroupStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, g := range f.groups {
		if g.ID == id {
			f.groups = append(f.groups[:i], f.groups[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

// fakeBuilder implements builder.ArtifactBuilder for testing.
type fakeBuilder struct {
	mu       sync.Mutex
	builds   []*builder.BuildStatus
	lastOpts builder.BuildOptions // lets tests assert on the generated cloud-config
}

func (f *fakeBuilder) Build(_ context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastOpts = opts
	status := &builder.BuildStatus{
		ID:    opts.ID,
		Phase: builder.BuildPending,
	}
	f.builds = append(f.builds, status)
	return status, nil
}

func (f *fakeBuilder) Status(_ context.Context, id string) (*builder.BuildStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.builds {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeBuilder) List(_ context.Context) ([]*builder.BuildStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.builds, nil
}

func (f *fakeBuilder) Cancel(_ context.Context, id string) error {
	return nil
}

// fakeSecureBootKeySetStore implements store.SecureBootKeySetStore for testing.
type fakeSecureBootKeySetStore struct {
	mu      sync.Mutex
	keySets []*store.SecureBootKeySet
	nextID  int
}

func (f *fakeSecureBootKeySetStore) Create(_ context.Context, ks *store.SecureBootKeySet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.keySets {
		if existing.Name == ks.Name {
			return fmt.Errorf("duplicate name %q", ks.Name)
		}
	}
	f.nextID++
	ks.ID = fmt.Sprintf("ks-%d", f.nextID)
	f.keySets = append(f.keySets, ks)
	return nil
}

func (f *fakeSecureBootKeySetStore) GetByID(_ context.Context, id string) (*store.SecureBootKeySet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ks := range f.keySets {
		if ks.ID == id {
			return ks, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeSecureBootKeySetStore) GetByName(_ context.Context, name string) (*store.SecureBootKeySet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ks := range f.keySets {
		if ks.Name == name {
			return ks, nil
		}
	}
	return nil, nil
}

func (f *fakeSecureBootKeySetStore) List(_ context.Context) ([]*store.SecureBootKeySet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keySets, nil
}

func (f *fakeSecureBootKeySetStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, ks := range f.keySets {
		if ks.ID == id {
			f.keySets = append(f.keySets[:i], f.keySets[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}
