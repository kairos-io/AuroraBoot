package gorm_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Gorm Store", func() {
	var (
		s   *gormstore.Store
		ctx context.Context
	)

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(":memory:")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	Describe("GroupStore", func() {
		It("creates a group and generates an ID", func() {
			g := &store.NodeGroup{Name: "production", Description: "prod env"}
			Expect(s.Create(ctx, g)).To(Succeed())
			Expect(g.ID).NotTo(BeEmpty())
		})

		It("lists groups", func() {
			Expect(s.Create(ctx, &store.NodeGroup{Name: "a"})).To(Succeed())
			Expect(s.Create(ctx, &store.NodeGroup{Name: "b"})).To(Succeed())

			groups, err := s.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(2))
		})

		It("gets by ID", func() {
			g := &store.NodeGroup{Name: "staging"}
			Expect(s.Create(ctx, g)).To(Succeed())

			found, err := s.GetByID(ctx, g.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Name).To(Equal("staging"))
		})

		It("gets by name", func() {
			g := &store.NodeGroup{Name: "dev"}
			Expect(s.Create(ctx, g)).To(Succeed())

			found, err := s.GetByName(ctx, "dev")
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ID).To(Equal(g.ID))
		})

		It("updates a group", func() {
			g := &store.NodeGroup{Name: "old-name"}
			Expect(s.Create(ctx, g)).To(Succeed())

			g.Name = "new-name"
			g.Description = "updated"
			Expect(s.Update(ctx, g)).To(Succeed())

			found, err := s.GetByID(ctx, g.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Name).To(Equal("new-name"))
			Expect(found.Description).To(Equal("updated"))
		})

		It("deletes a group", func() {
			g := &store.NodeGroup{Name: "to-delete"}
			Expect(s.Create(ctx, g)).To(Succeed())

			Expect(s.Delete(ctx, g.ID)).To(Succeed())

			_, err := s.GetByID(ctx, g.ID)
			Expect(err).To(HaveOccurred())
		})

		It("detaches member nodes when a group is deleted", func() {
			g := &store.NodeGroup{Name: "to-delete-with-nodes"}
			Expect(s.Create(ctx, g)).To(Succeed())

			n1 := &store.ManagedNode{MachineID: "del-n1", GroupID: g.ID}
			n2 := &store.ManagedNode{MachineID: "del-n2", GroupID: g.ID}
			Expect(s.Register(ctx, n1)).To(Succeed())
			Expect(s.Register(ctx, n2)).To(Succeed())

			Expect(s.Delete(ctx, g.ID)).To(Succeed())

			// Group gone.
			_, err := s.GetByID(ctx, g.ID)
			Expect(err).To(HaveOccurred())

			// Nodes still registered, but no longer attached to the deleted group.
			found1, err := s.NodeGetByID(ctx, n1.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found1.GroupID).To(BeEmpty())

			found2, err := s.NodeGetByID(ctx, n2.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found2.GroupID).To(BeEmpty())
		})

		It("rejects duplicate name", func() {
			Expect(s.Create(ctx, &store.NodeGroup{Name: "dup"})).To(Succeed())
			err := s.Create(ctx, &store.NodeGroup{Name: "dup"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("NodeStore", func() {
		It("registers a node with generated ID, API key, and Registered phase", func() {
			n := &store.ManagedNode{MachineID: "m1", Hostname: "host1"}
			Expect(s.Register(ctx, n)).To(Succeed())
			Expect(n.ID).NotTo(BeEmpty())
			Expect(n.APIKey).NotTo(BeEmpty())
			Expect(len(n.APIKey)).To(Equal(64)) // 32 bytes hex
			Expect(n.Phase).To(Equal(store.PhaseRegistered))
		})

		It("rejects duplicate machineID", func() {
			Expect(s.Register(ctx, &store.ManagedNode{MachineID: "m1"})).To(Succeed())
			err := s.Register(ctx, &store.ManagedNode{MachineID: "m1"})
			Expect(err).To(HaveOccurred())
		})

		It("gets by ID", func() {
			n := &store.ManagedNode{MachineID: "m2", Hostname: "h2"}
			Expect(s.Register(ctx, n)).To(Succeed())

			found, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Hostname).To(Equal("h2"))
		})

		It("gets by MachineID", func() {
			n := &store.ManagedNode{MachineID: "m3"}
			Expect(s.Register(ctx, n)).To(Succeed())

			found, err := s.GetByMachineID(ctx, "m3")
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ID).To(Equal(n.ID))
		})

		It("gets by APIKey", func() {
			n := &store.ManagedNode{MachineID: "m4"}
			Expect(s.Register(ctx, n)).To(Succeed())

			found, err := s.GetByAPIKey(ctx, n.APIKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ID).To(Equal(n.ID))
		})

		It("lists all nodes", func() {
			Expect(s.Register(ctx, &store.ManagedNode{MachineID: "a"})).To(Succeed())
			Expect(s.Register(ctx, &store.ManagedNode{MachineID: "b"})).To(Succeed())

			nodes, err := s.NodeList(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(2))
		})

		It("lists by group", func() {
			g := &store.NodeGroup{Name: "grp"}
			Expect(s.Create(ctx, g)).To(Succeed())

			n1 := &store.ManagedNode{MachineID: "n1", GroupID: g.ID}
			n2 := &store.ManagedNode{MachineID: "n2"}
			Expect(s.Register(ctx, n1)).To(Succeed())
			Expect(s.Register(ctx, n2)).To(Succeed())

			nodes, err := s.ListByGroup(ctx, g.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MachineID).To(Equal("n1"))
		})

		It("lists by labels with AND matching", func() {
			n1 := &store.ManagedNode{MachineID: "l1", Labels: map[string]string{"env": "prod", "tier": "web"}}
			n2 := &store.ManagedNode{MachineID: "l2", Labels: map[string]string{"env": "prod", "tier": "db"}}
			n3 := &store.ManagedNode{MachineID: "l3", Labels: map[string]string{"env": "staging"}}
			Expect(s.Register(ctx, n1)).To(Succeed())
			Expect(s.Register(ctx, n2)).To(Succeed())
			Expect(s.Register(ctx, n3)).To(Succeed())

			nodes, err := s.ListByLabels(ctx, map[string]string{"env": "prod", "tier": "web"})
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MachineID).To(Equal("l1"))
		})

		It("lists by selector combining group, labels, and nodeIDs", func() {
			g := &store.NodeGroup{Name: "sel-grp"}
			Expect(s.Create(ctx, g)).To(Succeed())

			n1 := &store.ManagedNode{MachineID: "s1", GroupID: g.ID, Labels: map[string]string{"role": "worker"}}
			n2 := &store.ManagedNode{MachineID: "s2", GroupID: g.ID, Labels: map[string]string{"role": "worker"}}
			n3 := &store.ManagedNode{MachineID: "s3", GroupID: g.ID, Labels: map[string]string{"role": "control"}}
			Expect(s.Register(ctx, n1)).To(Succeed())
			Expect(s.Register(ctx, n2)).To(Succeed())
			Expect(s.Register(ctx, n3)).To(Succeed())

			// group + labels + nodeIDs all AND'd
			nodes, err := s.ListBySelector(ctx, store.CommandSelector{
				GroupID: g.ID,
				Labels:  map[string]string{"role": "worker"},
				NodeIDs: []string{n1.ID},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].ID).To(Equal(n1.ID))
		})

		It("lists by selector with only group", func() {
			g := &store.NodeGroup{Name: "only-grp"}
			Expect(s.Create(ctx, g)).To(Succeed())

			n1 := &store.ManagedNode{MachineID: "og1", GroupID: g.ID}
			n2 := &store.ManagedNode{MachineID: "og2"}
			Expect(s.Register(ctx, n1)).To(Succeed())
			Expect(s.Register(ctx, n2)).To(Succeed())

			nodes, err := s.ListBySelector(ctx, store.CommandSelector{GroupID: g.ID})
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
		})

		It("updates heartbeat", func() {
			n := &store.ManagedNode{MachineID: "hb1"}
			Expect(s.Register(ctx, n)).To(Succeed())

			osRel := map[string]string{"name": "Kairos", "version": "1.0"}
			Expect(s.UpdateHeartbeat(ctx, n.ID, "v0.5.0", osRel)).To(Succeed())

			found, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.PhaseOnline))
			Expect(found.AgentVersion).To(Equal("v0.5.0"))
			Expect(found.OSRelease).To(HaveKeyWithValue("name", "Kairos"))
			Expect(found.LastHeartbeat).NotTo(BeNil())
		})

		It("updates phase", func() {
			n := &store.ManagedNode{MachineID: "ph1"}
			Expect(s.Register(ctx, n)).To(Succeed())

			Expect(s.UpdatePhase(ctx, n.ID, store.PhaseOffline)).To(Succeed())

			found, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.PhaseOffline))
		})

		It("sets group", func() {
			g := &store.NodeGroup{Name: "target-grp"}
			Expect(s.Create(ctx, g)).To(Succeed())

			n := &store.ManagedNode{MachineID: "sg1"}
			Expect(s.Register(ctx, n)).To(Succeed())

			Expect(s.SetGroup(ctx, n.ID, g.ID)).To(Succeed())

			found, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.GroupID).To(Equal(g.ID))
			Expect(found.Group).NotTo(BeNil())
			Expect(found.Group.Name).To(Equal("target-grp"))
		})

		It("sets labels", func() {
			n := &store.ManagedNode{MachineID: "sl1"}
			Expect(s.Register(ctx, n)).To(Succeed())

			labels := map[string]string{"env": "test", "team": "infra"}
			Expect(s.SetLabels(ctx, n.ID, labels)).To(Succeed())

			found, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Labels).To(Equal(labels))
		})

		It("deletes a node and its commands", func() {
			n := &store.ManagedNode{MachineID: "del1"}
			Expect(s.Register(ctx, n)).To(Succeed())

			cmd := &store.NodeCommand{ManagedNodeID: n.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.NodeDelete(ctx, n.ID)).To(Succeed())

			_, err := s.NodeGetByID(ctx, n.ID)
			Expect(err).To(HaveOccurred())

			cmds, err := s.ListByNode(ctx, n.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds).To(BeEmpty())
		})
	})

	Describe("CommandStore", func() {
		var node *store.ManagedNode

		BeforeEach(func() {
			node = &store.ManagedNode{MachineID: "cmd-node"}
			Expect(s.Register(ctx, node)).To(Succeed())
		})

		It("creates a command with Pending phase and generated ID", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdUpgrade}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())
			Expect(cmd.ID).NotTo(BeEmpty())
			Expect(cmd.Phase).To(Equal(store.CommandPending))
		})

		It("gets pending commands (not expired)", func() {
			future := time.Now().Add(1 * time.Hour)
			past := time.Now().Add(-1 * time.Hour)

			cmd1 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec, ExpiresAt: &future}
			cmd2 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec, ExpiresAt: &past}
			cmd3 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec} // no expiry
			Expect(s.CommandCreate(ctx, cmd1)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd2)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd3)).To(Succeed())

			// Mark cmd2 as delivered so it's not pending, but it's expired anyway
			// Actually cmd2 is still Pending but expired -- it should be excluded
			pending, err := s.GetPending(ctx, node.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(HaveLen(2)) // cmd1 (future expiry) and cmd3 (no expiry)
		})

		It("marks commands as delivered", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdReset}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.MarkDelivered(ctx, []string{cmd.ID})).To(Succeed())

			found, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.CommandDelivered))
			Expect(found.DeliveredAt).NotTo(BeNil())
		})

		It("updates status to completed with result and completedAt", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd.ID, store.CommandCompleted, "ok")).To(Succeed())

			found, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.CommandCompleted))
			Expect(found.Result).To(Equal("ok"))
			Expect(found.CompletedAt).NotTo(BeNil())
		})

		It("updates status to failed with result and completedAt", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd.ID, store.CommandFailed, "error occurred")).To(Succeed())

			found, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.CommandFailed))
			Expect(found.Result).To(Equal("error occurred"))
			Expect(found.CompletedAt).NotTo(BeNil())
		})

		It("updates status to running without completedAt", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd.ID, store.CommandRunning, "")).To(Succeed())

			found, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Phase).To(Equal(store.CommandRunning))
			Expect(found.CompletedAt).To(BeNil())
		})

		It("lists all commands by node", func() {
			Expect(s.CommandCreate(ctx, &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec})).To(Succeed())
			Expect(s.CommandCreate(ctx, &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdReset})).To(Succeed())

			cmds, err := s.ListByNode(ctx, node.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds).To(HaveLen(2))
		})

		It("gets command by ID", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdUpgrade, Args: map[string]string{"version": "1.2"}}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			found, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Command).To(Equal(store.CmdUpgrade))
			Expect(found.Args).To(HaveKeyWithValue("version", "1.2"))
		})
	})

	Describe("CommandStore Delete", func() {
		var node *store.ManagedNode

		BeforeEach(func() {
			node = &store.ManagedNode{MachineID: "cmd-del-node"}
			Expect(s.Register(ctx, node)).To(Succeed())
		})

		It("should delete a single command by ID", func() {
			cmd := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd)).To(Succeed())

			Expect(s.CommandDelete(ctx, cmd.ID)).To(Succeed())

			_, err := s.CommandGetByID(ctx, cmd.ID)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CommandStore DeleteTerminal", func() {
		var node *store.ManagedNode

		BeforeEach(func() {
			node = &store.ManagedNode{MachineID: "cmd-delterm-node"}
			Expect(s.Register(ctx, node)).To(Succeed())
		})

		It("should delete all Completed and Failed commands for a node", func() {
			cmd1 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			cmd2 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			cmd3 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			cmd4 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd1)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd2)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd3)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd4)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd1.ID, store.CommandCompleted, "done")).To(Succeed())
			Expect(s.UpdateStatus(ctx, cmd2.ID, store.CommandFailed, "error")).To(Succeed())
			Expect(s.UpdateStatus(ctx, cmd3.ID, store.CommandRunning, "")).To(Succeed())
			// cmd4 stays Pending

			Expect(s.CommandDeleteTerminal(ctx, node.ID)).To(Succeed())

			cmds, err := s.ListByNode(ctx, node.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds).To(HaveLen(2)) // Running + Pending remain
		})

		It("should NOT delete Pending or Running commands", func() {
			cmd1 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			cmd2 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd1)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd2)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd2.ID, store.CommandRunning, "")).To(Succeed())

			Expect(s.CommandDeleteTerminal(ctx, node.ID)).To(Succeed())

			cmds, err := s.ListByNode(ctx, node.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds).To(HaveLen(2))
		})

		It("should only affect the specified node's commands", func() {
			node2 := &store.ManagedNode{MachineID: "cmd-delterm-node2"}
			Expect(s.Register(ctx, node2)).To(Succeed())

			cmd1 := &store.NodeCommand{ManagedNodeID: node.ID, Command: store.CmdExec}
			cmd2 := &store.NodeCommand{ManagedNodeID: node2.ID, Command: store.CmdExec}
			Expect(s.CommandCreate(ctx, cmd1)).To(Succeed())
			Expect(s.CommandCreate(ctx, cmd2)).To(Succeed())

			Expect(s.UpdateStatus(ctx, cmd1.ID, store.CommandCompleted, "done")).To(Succeed())
			Expect(s.UpdateStatus(ctx, cmd2.ID, store.CommandCompleted, "done")).To(Succeed())

			Expect(s.CommandDeleteTerminal(ctx, node.ID)).To(Succeed())

			// node's commands should be gone
			cmds1, err := s.ListByNode(ctx, node.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds1).To(BeEmpty())

			// node2's commands should remain
			cmds2, err := s.ListByNode(ctx, node2.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmds2).To(HaveLen(1))
		})
	})

	Describe("ArtifactStore DeleteByPhase", func() {
		It("should delete all Error-phase artifacts", func() {
			rec1 := &store.ArtifactRecord{ID: "art-1", Phase: store.ArtifactError, BaseImage: "img1"}
			rec2 := &store.ArtifactRecord{ID: "art-2", Phase: store.ArtifactReady, BaseImage: "img2"}
			rec3 := &store.ArtifactRecord{ID: "art-3", Phase: store.ArtifactError, BaseImage: "img3"}
			Expect(s.ArtifactCreate(ctx, rec1)).To(Succeed())
			Expect(s.ArtifactCreate(ctx, rec2)).To(Succeed())
			Expect(s.ArtifactCreate(ctx, rec3)).To(Succeed())

			Expect(s.ArtifactDeleteByPhase(ctx, store.ArtifactError)).To(Succeed())

			records, err := s.ArtifactList(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(HaveLen(1))
			Expect(records[0].ID).To(Equal("art-2"))
		})

		It("should NOT delete Ready or Building artifacts", func() {
			rec1 := &store.ArtifactRecord{ID: "art-4", Phase: store.ArtifactReady, BaseImage: "img4"}
			rec2 := &store.ArtifactRecord{ID: "art-5", Phase: store.ArtifactBuilding, BaseImage: "img5"}
			Expect(s.ArtifactCreate(ctx, rec1)).To(Succeed())
			Expect(s.ArtifactCreate(ctx, rec2)).To(Succeed())

			Expect(s.ArtifactDeleteByPhase(ctx, store.ArtifactError)).To(Succeed())

			records, err := s.ArtifactList(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(HaveLen(2))
		})
	})

	Describe("ArtifactRecord.ExtensionHierarchies", func() {
		It("persists and reloads the hierarchies map", func() {
			rec := &store.ArtifactRecord{
				ID:        "art-hier-1",
				Phase:     store.ArtifactReady,
				BaseImage: "img-hier",
				ExtensionHierarchies: store.ExtensionHierarchies{
					Sysext: []string{"/opt", "/srv"},
				},
			}
			Expect(s.ArtifactCreate(ctx, rec)).To(Succeed())

			found, err := s.ArtifactGetByID(ctx, rec.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ExtensionHierarchies.Sysext).To(Equal([]string{"/opt", "/srv"}))
			Expect(found.ExtensionHierarchies.Confext).To(BeNil())
		})

		It("defaults to a zero value for legacy rows", func() {
			rec := &store.ArtifactRecord{
				ID:        "art-hier-2",
				Phase:     store.ArtifactReady,
				BaseImage: "img-hier-legacy",
			}
			Expect(s.ArtifactCreate(ctx, rec)).To(Succeed())

			found, err := s.ArtifactGetByID(ctx, rec.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.ExtensionHierarchies.Sysext).To(BeNil())
			Expect(found.ExtensionHierarchies.Confext).To(BeNil())
		})
	})

	Describe("ExtensionRecord schema", func() {
		It("creates the extensions table on AutoMigrate", func() {
			var count int64
			Expect(s.UnsafeDB().Model(&store.ExtensionRecord{}).Count(&count).Error).To(Succeed())
			Expect(count).To(BeZero())
		})

		It("round-trips Hierarchies", func() {
			rec := &store.ExtensionRecord{
				ID:          "e-1",
				Name:        "tailscale-agent",
				Type:        "sysext",
				Phase:       "Ready",
				Arch:        "amd64",
				Version:     "v1.74.0",
				SourceMode:  "image",
				SourceImage: "quay.io/myorg/tailscale:1.74",
				Hierarchies: []string{"/opt", "/srv"},
			}
			Expect(s.UnsafeDB().Create(rec).Error).To(Succeed())
			var got store.ExtensionRecord
			Expect(s.UnsafeDB().First(&got, "id = ?", "e-1").Error).To(Succeed())
			Expect(got.Hierarchies).To(Equal([]string{"/opt", "/srv"}))
		})
	})

	Describe("ArtifactExtensionBundle schema", func() {
		It("creates the table and round-trips an entry", func() {
			Expect(s.UnsafeDB().Create(&store.ArtifactExtensionBundle{
				ArtifactID:    "a-1",
				ExtensionName: "tailscale-agent",
				ExtensionType: "sysext",
				PinnedVersion: "",
				Order:         0,
			}).Error).To(Succeed())
			var got store.ArtifactExtensionBundle
			Expect(s.UnsafeDB().First(&got, "artifact_id = ? AND extension_name = ?", "a-1", "tailscale-agent").Error).To(Succeed())
			Expect(got.ExtensionType).To(Equal("sysext"))
		})

		It("rejects duplicate (artifact, name) pairs", func() {
			row := store.ArtifactExtensionBundle{ArtifactID: "a-2", ExtensionName: "x", ExtensionType: "sysext"}
			Expect(s.UnsafeDB().Create(&row).Error).To(Succeed())
			Expect(s.UnsafeDB().Create(&row).Error).To(HaveOccurred())
		})
	})

	Describe("NodeExtensionRow schema", func() {
		It("round-trips a row", func() {
			row := store.NodeExtensionRow{
				NodeID:      "n-1",
				Name:        "tailscale-agent",
				Type:        "sysext",
				Version:     "v1.74.0",
				BootState:   "common",
				InstalledAt: time.Now().UTC().Truncate(time.Second),
				ExtensionID: "e-1",
			}
			Expect(s.UnsafeDB().Create(&row).Error).To(Succeed())
			var got store.NodeExtensionRow
			Expect(s.UnsafeDB().First(&got, "node_id = ? AND name = ? AND type = ? AND boot_state = ?",
				"n-1", "tailscale-agent", "sysext", "common").Error).To(Succeed())
			Expect(got.Version).To(Equal("v1.74.0"))
		})

		It("rejects duplicate composite keys", func() {
			row := store.NodeExtensionRow{NodeID: "n-2", Name: "x", Type: "sysext", BootState: "common"}
			Expect(s.UnsafeDB().Create(&row).Error).To(Succeed())
			Expect(s.UnsafeDB().Create(&row).Error).To(HaveOccurred())
		})
	})

	Describe("ExtensionStore", func() {
		It("Create / GetByID round-trips", func() {
			ext := &store.ExtensionRecord{ID: "e-1", Name: "tailscale-agent", Type: "sysext", Phase: "Ready", Arch: "amd64", Version: "v1.74.0"}
			Expect(s.ExtensionCreate(ctx, ext)).To(Succeed())
			got, err := s.ExtensionGetByID(ctx, "e-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(got.Name).To(Equal("tailscale-agent"))
		})

		It("List orders by created_at descending", func() {
			Expect(s.ExtensionCreate(ctx, &store.ExtensionRecord{ID: "old", Name: "x", Type: "sysext", Phase: "Ready", CreatedAt: time.Now().Add(-1 * time.Hour)})).To(Succeed())
			Expect(s.ExtensionCreate(ctx, &store.ExtensionRecord{ID: "new", Name: "y", Type: "sysext", Phase: "Ready", CreatedAt: time.Now()})).To(Succeed())
			list, err := s.ExtensionList(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(list).To(HaveLen(2))
			Expect(list[0].ID).To(Equal("new"))
			Expect(list[1].ID).To(Equal("old"))
		})

		It("Delete removes one record", func() {
			Expect(s.ExtensionCreate(ctx, &store.ExtensionRecord{ID: "e-2", Name: "z", Type: "sysext", Phase: "Ready"})).To(Succeed())
			Expect(s.ExtensionDelete(ctx, "e-2")).To(Succeed())
			_, err := s.ExtensionGetByID(ctx, "e-2")
			Expect(err).To(HaveOccurred())
		})

		It("FindLatestReadyByName returns the newest Ready row by created_at", func() {
			old := &store.ExtensionRecord{ID: "f-old", Name: "tailscale", Type: "sysext", Version: "v1.72", Phase: "Ready", CreatedAt: time.Now().Add(-1 * time.Hour)}
			newer := &store.ExtensionRecord{ID: "f-new", Name: "tailscale", Type: "sysext", Version: "v1.74", Phase: "Ready", CreatedAt: time.Now()}
			errored := &store.ExtensionRecord{ID: "f-err", Name: "tailscale", Type: "sysext", Version: "v2.0", Phase: "Error", CreatedAt: time.Now().Add(1 * time.Hour)}
			for _, r := range []*store.ExtensionRecord{old, newer, errored} {
				Expect(s.ExtensionCreate(ctx, r)).To(Succeed())
			}
			got, derr := s.ExtensionFindLatestReadyByName(ctx, "sysext", "tailscale")
			Expect(derr).ToNot(HaveOccurred())
			Expect(got.ID).To(Equal("f-new"))
		})

		It("FindByNameAndVersion returns an exact match", func() {
			Expect(s.ExtensionCreate(ctx, &store.ExtensionRecord{ID: "v74", Name: "ts", Type: "sysext", Version: "v1.74", Phase: "Ready"})).To(Succeed())
			got, err := s.ExtensionFindByNameAndVersion(ctx, "sysext", "ts", "v1.74")
			Expect(err).ToNot(HaveOccurred())
			Expect(got.ID).To(Equal("v74"))
		})

		It("AppendLog appends chunks across calls", func() {
			Expect(s.ExtensionCreate(ctx, &store.ExtensionRecord{ID: "log-1", Name: "x", Type: "sysext", Phase: "Building"})).To(Succeed())
			Expect(s.ExtensionAppendLog(ctx, "log-1", "step 1...\n")).To(Succeed())
			Expect(s.ExtensionAppendLog(ctx, "log-1", "step 2...\n")).To(Succeed())
			got, err := s.ExtensionGetByID(ctx, "log-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(got.Logs).To(Equal("step 1...\nstep 2...\n"))
		})
	})

	Describe("ArtifactExtensionBundleStore", func() {
		It("ReplaceForArtifact replaces the entire set atomically", func() {
			Expect(s.BundleReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
				{ArtifactID: "a-1", ExtensionName: "tailscale", ExtensionType: "sysext", Order: 0},
				{ArtifactID: "a-1", ExtensionName: "fluent-bit", ExtensionType: "confext", Order: 1},
			})).To(Succeed())

			got, err := s.BundleListForArtifact(ctx, "a-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(HaveLen(2))

			// Replace with just one entry; the other should be dropped.
			Expect(s.BundleReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
				{ArtifactID: "a-1", ExtensionName: "tailscale", ExtensionType: "sysext"},
			})).To(Succeed())
			got, _ = s.BundleListForArtifact(ctx, "a-1")
			Expect(got).To(HaveLen(1))
			Expect(got[0].ExtensionName).To(Equal("tailscale"))
		})

		It("ArtifactsReferencingExtension lists artifacts that bundle a given name", func() {
			Expect(s.BundleReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
				{ArtifactID: "a-1", ExtensionName: "tailscale", ExtensionType: "sysext"},
			})).To(Succeed())
			Expect(s.BundleReplaceForArtifact(ctx, "a-2", []store.ArtifactExtensionBundle{
				{ArtifactID: "a-2", ExtensionName: "tailscale", ExtensionType: "sysext"},
				{ArtifactID: "a-2", ExtensionName: "fluent-bit", ExtensionType: "confext"},
			})).To(Succeed())

			refs, err := s.BundleArtifactsReferencingExtension(ctx, "tailscale")
			Expect(err).ToNot(HaveOccurred())
			Expect(refs).To(ConsistOf("a-1", "a-2"))
		})
	})

	Describe("NodeExtensionStore", func() {
		It("Upsert inserts a new row and updates an existing one", func() {
			Expect(s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{
				NodeID: "n-1", Name: "ts", Type: "sysext", BootState: "common",
				Version: "v1.72", ExtensionID: "e-old",
			})).To(Succeed())
			Expect(s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{
				NodeID: "n-1", Name: "ts", Type: "sysext", BootState: "common",
				Version: "v1.74", ExtensionID: "e-new",
			})).To(Succeed())
			rows, err := s.NodeExtensionListForNode(ctx, "n-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Version).To(Equal("v1.74"))
			Expect(rows[0].ExtensionID).To(Equal("e-new"))
		})

		It("DeleteByName drops all rows for a name on that node", func() {
			_ = s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{NodeID: "n-2", Name: "ts", Type: "sysext", BootState: "common"})
			_ = s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{NodeID: "n-2", Name: "ts", Type: "sysext", BootState: "active"})
			Expect(s.NodeExtensionDeleteByName(ctx, "n-2", "sysext", "ts")).To(Succeed())
			rows, _ := s.NodeExtensionListForNode(ctx, "n-2")
			Expect(rows).To(BeEmpty())
		})

		It("DeleteByScope drops the row for a specific scope only", func() {
			_ = s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{NodeID: "n-3", Name: "ts", Type: "sysext", BootState: "common"})
			_ = s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{NodeID: "n-3", Name: "ts", Type: "sysext", BootState: "active"})
			Expect(s.NodeExtensionDeleteByScope(ctx, "n-3", "sysext", "ts", "active")).To(Succeed())
			rows, _ := s.NodeExtensionListForNode(ctx, "n-3")
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].BootState).To(Equal("common"))
		})

		It("ListForExtensionByName aggregates rows across nodes", func() {
			for _, n := range []string{"n-4", "n-5", "n-6"} {
				_ = s.NodeExtensionUpsert(ctx, &store.NodeExtensionRow{NodeID: n, Name: "ts2", Type: "sysext", BootState: "common"})
			}
			rows, err := s.NodeExtensionListForExtensionByName(ctx, "sysext", "ts2")
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(3))
		})
	})
})
