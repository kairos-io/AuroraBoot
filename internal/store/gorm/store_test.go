package gorm_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/store"
	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
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
})
