import { useEffect, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { getGroup, deleteGroup, sendGroupCommand, type Group } from "@/api/groups";
import { listNodes, type Node } from "@/api/nodes";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { NodeTable } from "@/components/NodeTable";
import { PageHeader } from "@/components/PageHeader";
import { CommandDialog } from "@/components/CommandDialog";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ChevronDown, Download, Trash2 } from "lucide-react";
import { toast } from "@/hooks/useToast";

export function GroupDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [group, setGroup] = useState<Group | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [cmdOpen, setCmdOpen] = useState(false);
  const [quickCommand, setQuickCommand] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  async function handleDelete() {
    if (!id || !group) return;
    setConfirmDelete(false);
    try {
      await deleteGroup(id);
      toast(`Deleted group "${group.name}"`, "success");
      navigate("/groups");
    } catch (err) {
      toast(`Failed to delete group: ${(err as Error).message}`, "error");
    }
  }

  useEffect(() => {
    if (!id) return;
    getGroup(id).then(setGroup).catch(() => {});
    listNodes({ group_id: id }).then(setNodes).catch(() => {});
  }, [id]);

  async function handleCommand(command: string, args: Record<string, unknown>) {
    if (!id) return;
    await sendGroupCommand(id, command, args);
    setCmdOpen(false);
    setQuickCommand(null);
  }

  function handleQuickAction(cmd: string) {
    setQuickCommand(cmd);
    setCmdOpen(true);
  }

  if (!group) {
    return <div className="text-muted-foreground">Loading...</div>;
  }

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
        <button onClick={() => navigate("/groups")} className="hover:text-foreground">Groups</button>
        <span>/</span>
        <span className="text-foreground">{group.name}</span>
      </div>

      <PageHeader title={group.name} description={group.description}>
        <Button variant="outline" asChild>
          <Link to={`/import?group=${id}`}>
            <Download className="h-4 w-4 mr-2" />
            Import Nodes
          </Link>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button className="bg-[#EE5007] hover:bg-[#FF7442] text-white">
              Bulk Actions
              <ChevronDown className="h-4 w-4 ml-2" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onClick={() => handleQuickAction("upgrade")}>
              Upgrade
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handleQuickAction("reset")}>
              Reset
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handleQuickAction("apply-config")}>
              Apply Config
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="outline"
          className="text-red-500 hover:text-red-700"
          onClick={() => setConfirmDelete(true)}
        >
          <Trash2 className="h-4 w-4 mr-2" />
          Delete
        </Button>
      </PageHeader>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Delete Group"
        description={
          group.node_count > 0
            ? `Delete "${group.name}"? ${group.node_count} node(s) will be moved out of this group (they stay registered).`
            : `Delete "${group.name}"? This group has no nodes.`
        }
        confirmLabel="Delete"
        destructive
        onConfirm={handleDelete}
      />

      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-sm font-medium">Group Info</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <dt className="text-muted-foreground">ID</dt>
              <dd className="font-mono">{group.id}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Node Count</dt>
              <dd>{group.node_count}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <h2 className="text-xl font-semibold mb-4">Nodes</h2>
      <NodeTable nodes={nodes} />

      <CommandDialog
        open={cmdOpen}
        onOpenChange={(open) => {
          setCmdOpen(open);
          if (!open) setQuickCommand(null);
        }}
        onSubmit={handleCommand}
        title={`Send command · ${group.name}`}
        defaultCommand={quickCommand as "upgrade" | "reset" | "apply-config" | null}
      />
    </div>
  );
}
