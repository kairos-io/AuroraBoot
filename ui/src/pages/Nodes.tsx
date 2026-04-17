import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { listNodes, sendBulkCommand, type Node, type NodeListParams } from "@/api/nodes";
import { listGroups, type Group } from "@/api/groups";
import { NodeTable } from "@/components/NodeTable";
import { PageHeader } from "@/components/PageHeader";
import { CommandDialog } from "@/components/CommandDialog";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Terminal } from "lucide-react";

export function Nodes() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupFilter, setGroupFilter] = useState("__all__");
  const [labelFilter, setLabelFilter] = useState("");
  const [phaseFilter, setPhaseFilter] = useState("__all__");
  const [hostnameSearch, setHostnameSearch] = useState("");
  const [bulkCmdOpen, setBulkCmdOpen] = useState(false);
  const [confirmState, setConfirmState] = useState<{ open: boolean; action: () => void }>({ open: false, action: () => {} });
  const navigate = useNavigate();

  const load = useCallback(() => {
    const params: NodeListParams = {};
    if (groupFilter && groupFilter !== "__all__") params.group_id = groupFilter;
    if (labelFilter) params.label = labelFilter;
    if (phaseFilter && phaseFilter !== "__all__") params.phase = phaseFilter;
    listNodes(params).then(setNodes).catch(() => {});
  }, [groupFilter, labelFilter, phaseFilter]);

  useEffect(() => {
    listGroups().then(setGroups).catch(() => {});
  }, []);

  // Poll the node list so newly-registered machines appear without a
  // manual refresh. Five seconds matches the kairos-agent's default
  // reconnect backoff, so a freshly-booted node typically shows up on
  // the next tick after it phones home. The interval re-arms whenever
  // the filters change (via `load`'s deps), so we always poll with the
  // currently-selected filter set.
  useEffect(() => {
    load();
    const id = setInterval(load, 5000);
    return () => clearInterval(id);
  }, [load]);

  const filteredNodes = nodes.filter(
    (n) => !hostnameSearch || n.hostname.toLowerCase().includes(hostnameSearch.toLowerCase())
  );

  function handleBulkSubmit(command: string, args: Record<string, unknown>) {
    const selector: { groupID?: string; labels?: Record<string, string> } = {};

    if (groupFilter && groupFilter !== "__all__") {
      selector.groupID = groupFilter;
    }
    if (labelFilter) {
      const [k, v] = labelFilter.split("=");
      if (k) {
        selector.labels = { [k.trim()]: (v || "").trim() };
      }
    }

    // If no filters active, require confirmation
    if (!selector.groupID && !selector.labels) {
      setConfirmState({
        open: true,
        action: () => {
          const nodeIDs = nodes.map((n) => n.id);
          sendBulkCommand({ nodeIDs }, command, args).catch(() => {});
          setBulkCmdOpen(false);
        },
      });
      return;
    }

    sendBulkCommand(selector, command, args).catch(() => {});
    setBulkCmdOpen(false);
  }

  return (
    <div>
      <PageHeader title="Nodes" description="Manage your registered machines">
        <Button
          className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
          disabled={nodes.length === 0}
          onClick={() => setBulkCmdOpen(true)}
        >
          <Terminal className="h-4 w-4 mr-2" />
          Send Command{nodes.length > 0 ? ` to ${nodes.length} nodes` : ""}
        </Button>
      </PageHeader>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="grid gap-2">
          <Label>Hostname</Label>
          <Input
            placeholder="Search by hostname..."
            value={hostnameSearch}
            onChange={(e) => setHostnameSearch(e.target.value)}
          />
        </div>
        <div className="grid gap-2">
          <Label>Group</Label>
          <Select value={groupFilter} onValueChange={setGroupFilter}>
            <SelectTrigger>
              <SelectValue placeholder="All groups" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">All groups</SelectItem>
              {groups.map((g) => (
                <SelectItem key={g.id} value={g.id}>
                  {g.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label>Label Filter</Label>
          <Input
            placeholder="e.g. role=worker"
            value={labelFilter}
            onChange={(e) => setLabelFilter(e.target.value)}
          />
        </div>
        <div className="grid gap-2">
          <Label>Phase</Label>
          <Select value={phaseFilter} onValueChange={setPhaseFilter}>
            <SelectTrigger>
              <SelectValue placeholder="All phases" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">All phases</SelectItem>
              <SelectItem value="Online">Online</SelectItem>
              <SelectItem value="Offline">Offline</SelectItem>
              <SelectItem value="Pending">Pending</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <NodeTable nodes={filteredNodes} emptyAction={() => navigate("/import")} />

      <CommandDialog
        open={bulkCmdOpen}
        onOpenChange={setBulkCmdOpen}
        onSubmit={handleBulkSubmit}
        title={`Send Command to ${nodes.length} node${nodes.length !== 1 ? "s" : ""}`}
      />

      <ConfirmDialog
        open={confirmState.open}
        onOpenChange={(open) => setConfirmState(prev => ({ ...prev, open }))}
        title="Send to All Nodes"
        description={`This will send the command to ALL ${nodes.length} nodes. Are you sure you want to continue?`}
        confirmLabel="Send to All"
        onConfirm={confirmState.action}
      />
    </div>
  );
}
