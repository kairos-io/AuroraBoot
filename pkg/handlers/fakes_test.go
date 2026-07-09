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

// newFakeCommandStore returns an empty fakeCommandStore.
func newFakeCommandStore() *fakeCommandStore { return &fakeCommandStore{} }

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
			// Return a copy, not the stored pointer: real gorm GetPending
			// materializes a fresh struct per query, so callers (e.g. concurrent
			// polls in GetCommands) must each get their own object to mutate.
			cp := *cmd
			result = append(result, &cp)
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

func (f *fakeCommandStore) ClaimForDelivery(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cmd := range f.cmds {
		if cmd.ID == id && cmd.Phase == store.CommandPending {
			cmd.Phase = store.CommandDelivered
			return true, nil
		}
	}
	return false, nil
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

func (f *fakeCommandStore) UpdateStatusForNode(_ context.Context, id string, nodeID string, phase string, result string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cmd := range f.cmds {
		if cmd.ID == id && cmd.ManagedNodeID == nodeID {
			cmd.Phase = phase
			cmd.Result = result
			return nil
		}
	}
	return store.ErrCommandNotFound
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
	mu        sync.Mutex
	records   []*store.ArtifactRecord
	lastSaved *store.ArtifactRecord // most recent Create/Update target
}

// newFakeArtifactStore returns an empty fakeArtifactStore.
func newFakeArtifactStore() *fakeArtifactStore { return &fakeArtifactStore{} }

func (f *fakeArtifactStore) Create(_ context.Context, rec *store.ArtifactRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rec)
	f.lastSaved = rec
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
	f.lastSaved = rec
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
	// buildErr, when set, is returned from Build instead of starting a build.
	// Tests use it to exercise the handler's error-to-status mapping without a
	// real builder.
	buildErr error
}

func (f *fakeBuilder) Build(_ context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastOpts = opts
	if f.buildErr != nil {
		// Mirror the real builder: validation failures are rejected before any
		// build state is recorded.
		return nil, f.buildErr
	}
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

// fakeSettingsStore implements store.SettingsStore for testing.
type fakeSettingsStore struct {
	mu     sync.Mutex
	values map[string]string
	getErr error // when set, Get returns this error (to exercise fail-closed paths)
	setErr error // when set, Set returns this error
	allErr error // when set, GetAll returns this error
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{values: map[string]string{}}
}

func (f *fakeSettingsStore) Get(_ context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettingsStore) Set(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.values[key] = value
	return nil
}

func (f *fakeSettingsStore) GetAll(_ context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allErr != nil {
		return nil, f.allErr
	}
	out := make(map[string]string, len(f.values))
	for k, v := range f.values {
		out[k] = v
	}
	return out, nil
}

// --- fakeNodeExtensionStore --------------------------------------------

type fakeNodeExtensionStore struct {
	mu   sync.Mutex
	rows []store.NodeExtensionRow
}

func newFakeNodeExtensionStore() *fakeNodeExtensionStore {
	return &fakeNodeExtensionStore{}
}

func (f *fakeNodeExtensionStore) Upsert(_ context.Context, row *store.NodeExtensionRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.NodeID == row.NodeID && r.Name == row.Name && r.Type == row.Type && r.BootState == row.BootState {
			f.rows[i] = *row
			return nil
		}
	}
	f.rows = append(f.rows, *row)
	return nil
}

func (f *fakeNodeExtensionStore) ListForNode(_ context.Context, nodeID string) ([]store.NodeExtensionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []store.NodeExtensionRow{}
	for _, r := range f.rows {
		if r.NodeID == nodeID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeNodeExtensionStore) ListForExtensionByName(_ context.Context, extType, name string) ([]store.NodeExtensionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []store.NodeExtensionRow{}
	for _, r := range f.rows {
		if r.Type == extType && r.Name == name {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeNodeExtensionStore) DeleteByScope(_ context.Context, nodeID, extType, name, bootState string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.rows[:0]
	for _, r := range f.rows {
		if r.NodeID == nodeID && r.Type == extType && r.Name == name && r.BootState == bootState {
			continue
		}
		out = append(out, r)
	}
	f.rows = out
	return nil
}

func (f *fakeNodeExtensionStore) DeleteByName(_ context.Context, nodeID, extType, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.rows[:0]
	for _, r := range f.rows {
		if r.NodeID == nodeID && r.Type == extType && r.Name == name {
			continue
		}
		out = append(out, r)
	}
	f.rows = out
	return nil
}

// --- fakeExtensionBuilder -----------------------------------------------

type fakeExtensionBuilder struct {
	mu       sync.Mutex
	lastOpts builder.ExtensionBuildOptions
	buildErr error
	cancels  []string
}

func (f *fakeExtensionBuilder) Build(_ context.Context, opts builder.ExtensionBuildOptions) (*builder.ExtensionBuildStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastOpts = opts
	if f.buildErr != nil {
		return nil, f.buildErr
	}
	return &builder.ExtensionBuildStatus{ID: opts.ID, Phase: builder.BuildPending}, nil
}
func (f *fakeExtensionBuilder) Status(_ context.Context, id string) (*builder.ExtensionBuildStatus, error) {
	return &builder.ExtensionBuildStatus{ID: id, Phase: builder.BuildReady}, nil
}
func (f *fakeExtensionBuilder) List(_ context.Context) ([]*builder.ExtensionBuildStatus, error) {
	return nil, nil
}
func (f *fakeExtensionBuilder) Cancel(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
	return nil
}

// --- fakeExtensionStore -------------------------------------------------

type fakeExtensionStore struct {
	mu   sync.Mutex
	rows map[string]*store.ExtensionRecord
}

func newFakeExtensionStore() *fakeExtensionStore {
	return &fakeExtensionStore{rows: map[string]*store.ExtensionRecord{}}
}

func (f *fakeExtensionStore) Create(_ context.Context, r *store.ExtensionRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.rows[r.ID] = &cp
	return nil
}
func (f *fakeExtensionStore) GetByID(_ context.Context, id string) (*store.ExtensionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeExtensionStore) List(context.Context) ([]store.ExtensionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.ExtensionRecord, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeExtensionStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
}
func (f *fakeExtensionStore) FindLatestReadyByName(_ context.Context, extType, name string) (*store.ExtensionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var newest *store.ExtensionRecord
	for _, r := range f.rows {
		if r.Type != extType || r.Name != name || r.Phase != "Ready" {
			continue
		}
		if newest == nil || r.CreatedAt.After(newest.CreatedAt) {
			cp := *r
			newest = &cp
		}
	}
	if newest == nil {
		return nil, fmt.Errorf("not found")
	}
	return newest, nil
}
func (f *fakeExtensionStore) FindByNameAndVersion(_ context.Context, extType, name, version string) (*store.ExtensionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Type == extType && r.Name == name && r.Version == version && r.Phase == "Ready" {
			cp := *r
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeExtensionStore) AppendLog(_ context.Context, id, chunk string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		r.Logs += chunk
	}
	return nil
}

// --- fakeBundleStore ----------------------------------------------------

type fakeBundleStore struct {
	mu     sync.Mutex
	rowsBy map[string][]store.ArtifactExtensionBundle // keyed by artifact id
	byName map[string][]string                        // extension name -> artifact ids
}

func newFakeBundleStore() *fakeBundleStore {
	return &fakeBundleStore{
		rowsBy: map[string][]store.ArtifactExtensionBundle{},
		byName: map[string][]string{},
	}
}

func (f *fakeBundleStore) ListForArtifact(_ context.Context, artifactID string) ([]store.ArtifactExtensionBundle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.ArtifactExtensionBundle(nil), f.rowsBy[artifactID]...), nil
}
func (f *fakeBundleStore) ReplaceForArtifact(_ context.Context, artifactID string, entries []store.ArtifactExtensionBundle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, old := range f.rowsBy[artifactID] {
		f.removeNameMapLocked(old.ExtensionName, artifactID)
	}
	f.rowsBy[artifactID] = append([]store.ArtifactExtensionBundle(nil), entries...)
	for _, e := range entries {
		f.byName[e.ExtensionName] = append(f.byName[e.ExtensionName], artifactID)
	}
	return nil
}
func (f *fakeBundleStore) ArtifactsReferencingExtension(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.byName[name]...), nil
}
func (f *fakeBundleStore) removeNameMapLocked(name, artifactID string) {
	ids := f.byName[name]
	out := ids[:0]
	for _, id := range ids {
		if id != artifactID {
			out = append(out, id)
		}
	}
	f.byName[name] = out
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
