import { useEffect, useState, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { getNode, sendCommand, setLabels, setGroup, type Node } from "@/api/nodes";
import { DecommissionDialog } from "@/components/DecommissionDialog";
import { listNodeCommands, deleteCommand, clearCommandHistory, type Command } from "@/api/commands";
import { listGroups, type Group } from "@/api/groups";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { StatusBadge } from "@/components/StatusBadge";
import { PageHeader } from "@/components/PageHeader";
import { CommandDialog } from "@/components/CommandDialog";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { useUIWebSocket } from "@/hooks/useUIWebSocket";
import { Trash2, ChevronDown, ChevronRight, Terminal } from "lucide-react";
import { ansiToHtml } from "@/lib/ansi";

function timeAgo(dateStr: string): string {
  if (!dateStr) return "Never";
  const diff = Date.now() - new Date(dateStr).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function NodeDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [node, setNode] = useState<Node | null>(null);
  const [cmdOpen, setCmdOpen] = useState(false);
  const [labelInput, setLabelInput] = useState("");
  const [editingLabels, setEditingLabels] = useState(false);
  const [commands, setCommands] = useState<Command[]>([]);
  const [expandedCmds, setExpandedCmds] = useState<Set<string>>(new Set());
  const [groups, setGroups] = useState<Group[]>([]);
  const [confirmState, setConfirmState] = useState<{ open: boolean; action: () => void; title: string; description: string }>({ open: false, action: () => {}, title: "", description: "" });

  const fetchCommands = useCallback(() => {
    if (!id) return;
    listNodeCommands(id).then((cmds) => {
      setCommands(cmds);
      // Auto-expand running commands
      const running = new Set(
        cmds.filter((c) => c.phase === "Running" || c.phase === "Pending").map((c) => c.id)
      );
      setExpandedCmds((prev) => {
        const next = new Set(prev);
        running.forEach((rid) => next.add(rid));
        return next;
      });
    }).catch(() => {});
  }, [id]);

  const fetchNode = useCallback(() => {
    if (!id) return;
    getNode(id).then((n) => {
      setNode(n);
      setLabelInput(
        Object.entries(n.labels || {})
          .map(([k, v]) => `${k}=${v}`)
          .join(", ")
      );
    }).catch(() => {});
  }, [id]);

  useEffect(() => {
    fetchNode();
    fetchCommands();
    listGroups().then(setGroups).catch(() => {});
  }, [fetchNode, fetchCommands]);

  // Fallback polling every 10s
  useEffect(() => {
    const interval = setInterval(fetchCommands, 10000);
    return () => clearInterval(interval);
  }, [fetchCommands]);

  // Live updates via WebSocket
  useUIWebSocket((msg) => {
    if (msg.type === "command_update" && msg.data) {
      setCommands((prev) =>
        prev.map((cmd) =>
          cmd.id === msg.data.id
            ? { ...cmd, phase: msg.data.phase ?? cmd.phase, result: msg.data.result ?? cmd.result }
            : cmd
        )
      );
    }
  });

  async function handleCommand(command: string, args: Record<string, unknown>) {
    if (!id) return;
    await sendCommand(id, command, args);
    setCmdOpen(false);
    fetchCommands();
  }

  async function handleDeleteCommand(commandID: string) {
    if (!id) return;
    await deleteCommand(id, commandID);
    setCommands((prev) => prev.filter((c) => c.id !== commandID));
  }

  async function handleClearHistory() {
    if (!id) return;
    setConfirmState({
      open: true,
      title: "Clear Command History",
      description: "Clear all completed and failed commands? This cannot be undone.",
      action: async () => {
        await clearCommandHistory(id);
        fetchCommands();
      },
    });
  }

  function toggleExpanded(commandID: string) {
    setExpandedCmds((prev) => {
      const next = new Set(prev);
      if (next.has(commandID)) {
        next.delete(commandID);
      } else {
        next.add(commandID);
      }
      return next;
    });
  }

  const sortedCommands = [...commands].sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  );

  async function handleSaveLabels() {
    if (!id) return;
    const labels: Record<string, string> = {};
    labelInput
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .forEach((pair) => {
        const [k, v] = pair.split("=");
        if (k) labels[k.trim()] = (v || "").trim();
      });
    const updated = await setLabels(id, labels);
    setNode(updated);
    setEditingLabels(false);
  }

  const [decommissionOpen, setDecommissionOpen] = useState(false);

  if (!node) {
    return <div className="text-muted-foreground">Loading...</div>;
  }

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
        <button onClick={() => navigate("/nodes")} className="hover:text-foreground">Nodes</button>
        <span>/</span>
        <span className="text-foreground">{node.hostname}</span>
      </div>

      <PageHeader title={node.hostname}>
        <StatusBadge status={node.phase} />
        <Button className="bg-[#EE5007] hover:bg-[#FF7442] text-white" onClick={() => setCmdOpen(true)}>
          Send Command
        </Button>
        <Button
          variant="destructive"
          size="icon"
          onClick={() => setDecommissionOpen(true)}
          aria-label="Decommission node"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </PageHeader>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Node Info</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-3 text-sm">
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Machine ID</dt>
                <dd className="font-mono text-xs">{node.machineID}</dd>
              </div>
              <Separator />
              <div className="flex justify-between items-center">
                <dt className="text-muted-foreground">Group</dt>
                <dd>
                  <Select
                    value={node.groupID || "__none__"}
                    onValueChange={async (v) => {
                      await setGroup(id!, v === "__none__" ? "" : v);
                      fetchNode();
                    }}
                  >
                    <SelectTrigger className="h-7 w-40 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__none__">None</SelectItem>
                      {groups.map((g) => (
                        <SelectItem key={g.id} value={g.id}>
                          {g.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </dd>
              </div>
              <Separator />
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Phase</dt>
                <dd><StatusBadge status={node.phase} /></dd>
              </div>
              <Separator />
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Agent Version</dt>
                <dd className="text-xs">{node.agentVersion || "-"}</dd>
              </div>
              <Separator />
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Last Heartbeat</dt>
                <dd>{timeAgo(node.lastHeartbeat || "")}</dd>
              </div>
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">Labels</CardTitle>
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  editingLabels ? handleSaveLabels() : setEditingLabels(true)
                }
              >
                {editingLabels ? "Save" : "Edit"}
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            {editingLabels ? (
              <div className="grid gap-2">
                <Label>Labels (comma-separated key=value pairs)</Label>
                <Input
                  value={labelInput}
                  onChange={(e) => setLabelInput(e.target.value)}
                  placeholder="role=worker, env=prod"
                />
              </div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {Object.entries(node.labels || {}).length === 0 ? (
                  <span className="text-muted-foreground text-sm">
                    No labels
                  </span>
                ) : (
                  Object.entries(node.labels || {}).map(([k, v]) => (
                    <Badge key={k} variant="secondary">
                      {k}={v}
                    </Badge>
                  ))
                )}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* System Info */}
      {node.osRelease && Object.keys(node.osRelease).length > 0 && (
        <Card className="mt-6">
          <CardHeader>
            <CardTitle className="text-sm font-medium">System Info</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              {node.osRelease.PRETTY_NAME && (
                <div>
                  <span className="text-muted-foreground text-xs">OS</span>
                  <p className="mt-0.5">{node.osRelease.PRETTY_NAME}</p>
                </div>
              )}
              {node.osRelease.KAIROS_VERSION && (
                <div>
                  <span className="text-muted-foreground text-xs">Kairos</span>
                  <p className="mt-0.5">{node.osRelease.KAIROS_VERSION}</p>
                </div>
              )}
              {node.osRelease.KAIROS_FLAVOR && (
                <div>
                  <span className="text-muted-foreground text-xs">Flavor</span>
                  <p className="mt-0.5">{node.osRelease.KAIROS_FLAVOR}</p>
                </div>
              )}
              {node.osRelease.KERNEL && (
                <div>
                  <span className="text-muted-foreground text-xs">Kernel</span>
                  <p className="mt-0.5 font-mono text-xs">{node.osRelease.KERNEL}</p>
                </div>
              )}
              {node.osRelease.ARCH && (
                <div>
                  <span className="text-muted-foreground text-xs">Architecture</span>
                  <p className="mt-0.5">{node.osRelease.ARCH}</p>
                </div>
              )}
              {node.osRelease.CPU_COUNT && (
                <div>
                  <span className="text-muted-foreground text-xs">CPU</span>
                  <p className="mt-0.5">{node.osRelease.CPU_COUNT} cores</p>
                </div>
              )}
              {node.osRelease.MEM_TOTAL && (
                <div>
                  <span className="text-muted-foreground text-xs">Memory</span>
                  <p className="mt-0.5">{node.osRelease.MEM_TOTAL}</p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Command History */}
      <Card className="mt-6">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm font-medium">Commands</CardTitle>
            <div className="flex gap-2">
              <Button size="sm" className="bg-[#EE5007] hover:bg-[#FF7442] text-white" onClick={() => setCmdOpen(true)}>
                Send New
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {sortedCommands.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-8">
              <Terminal className="h-8 w-8 text-muted-foreground/50" />
              <p className="text-sm font-medium text-muted-foreground">No commands yet</p>
              <p className="text-xs text-muted-foreground/70">
                Send a command to interact with this node.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {sortedCommands.map((cmd) => {
                const isExpanded = expandedCmds.has(cmd.id);
                return (
                  <div key={cmd.id} className="border rounded-md">
                    <div
                      className="flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-muted/50"
                      onClick={() => toggleExpanded(cmd.id)}
                    >
                      {isExpanded ? (
                        <ChevronDown className="h-4 w-4 shrink-0" />
                      ) : (
                        <ChevronRight className="h-4 w-4 shrink-0" />
                      )}
                      <Badge variant="outline" className="font-mono text-xs">
                        {cmd.command}
                      </Badge>
                      <StatusBadge status={cmd.phase} />
                      <span className="text-xs text-muted-foreground ml-auto">
                        {timeAgo(cmd.createdAt)}
                      </span>
                      {cmd.phase !== "Running" && cmd.phase !== "Pending" && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteCommand(cmd.id);
                          }}
                        >
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      )}
                    </div>
                    {isExpanded && (
                      <div className="px-3 pb-3">
                        {cmd.args && Object.keys(cmd.args).length > 0 && (
                          <div className="flex flex-wrap gap-1 mb-2">
                            {Object.entries(cmd.args).map(([k, v]) => (
                              <Badge key={k} variant="secondary" className="text-xs">
                                {k}: {v}
                              </Badge>
                            ))}
                          </div>
                        )}
                        <div className="terminal-output font-mono text-xs p-3 rounded max-h-48 overflow-y-auto whitespace-pre-wrap">
                          {cmd.result ? (
                            <span dangerouslySetInnerHTML={{ __html: ansiToHtml(cmd.result) }} />
                          ) : (
                            "Waiting for output..."
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
          {sortedCommands.length > 0 && (
            <div className="mt-4 flex justify-end">
              <Button variant="outline" size="sm" onClick={handleClearHistory}>
                Clear History
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <CommandDialog
        open={cmdOpen}
        onOpenChange={setCmdOpen}
        onSubmit={handleCommand}
        title={`Send command · ${node.hostname}`}
      />

      <ConfirmDialog
        open={confirmState.open}
        onOpenChange={(open) => setConfirmState(prev => ({ ...prev, open }))}
        title={confirmState.title}
        description={confirmState.description}
        confirmLabel="Confirm"
        destructive
        onConfirm={confirmState.action}
      />

      <DecommissionDialog
        open={decommissionOpen}
        onOpenChange={setDecommissionOpen}
        node={node}
        onDeleted={() => navigate("/nodes")}
      />
    </div>
  );
}
