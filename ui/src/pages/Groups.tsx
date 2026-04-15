import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { listGroups, createGroup, deleteGroup, type Group } from "@/api/groups";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageHeader } from "@/components/PageHeader";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Plus, FolderTree, Trash2 } from "lucide-react";
import { toast } from "@/hooks/useToast";

export function Groups() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [confirmTarget, setConfirmTarget] = useState<Group | null>(null);
  const navigate = useNavigate();

  function load() {
    listGroups().then(setGroups).catch(() => {});
  }

  useEffect(() => {
    load();
  }, []);

  async function handleCreate() {
    if (!name.trim()) return;
    await createGroup({ name, description });
    setName("");
    setDescription("");
    setDialogOpen(false);
    load();
  }

  async function handleConfirmDelete() {
    if (!confirmTarget) return;
    const target = confirmTarget;
    setConfirmTarget(null);
    try {
      await deleteGroup(target.id);
      toast(`Deleted group "${target.name}"`, "success");
      load();
    } catch (err) {
      toast(`Failed to delete group: ${(err as Error).message}`, "error");
    }
  }

  return (
    <div>
      <PageHeader title="Groups" description="Organize nodes into logical groups">
        <Button onClick={() => setDialogOpen(true)}>
          <Plus className="h-4 w-4 mr-2" />
          Create Group
        </Button>
      </PageHeader>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Description</TableHead>
            <TableHead>Nodes</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.length === 0 ? (
            <TableRow>
              <TableCell colSpan={4} className="text-center py-12">
                <div className="flex flex-col items-center gap-3 py-16">
                  <FolderTree className="h-12 w-12 text-muted-foreground/30" />
                  <div className="text-center">
                    <p className="font-medium">No groups</p>
                    <p className="text-sm text-muted-foreground mt-1">
                      Create a group to organize your nodes by environment or role.
                    </p>
                  </div>
                  <Button className="mt-2" onClick={() => setDialogOpen(true)}>
                    <Plus className="h-4 w-4 mr-2" /> Create Group
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ) : (
            groups.map((group) => (
              <TableRow
                key={group.id}
                className="cursor-pointer hover:bg-[#EE5007]/5"
                onClick={() => navigate(`/groups/${group.id}`)}
              >
                <TableCell className="font-medium">{group.name}</TableCell>
                <TableCell>{group.description || "-"}</TableCell>
                <TableCell>{group.node_count}</TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-red-500 hover:text-red-700"
                    title="Delete group"
                    onClick={() => setConfirmTarget(group)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      <ConfirmDialog
        open={!!confirmTarget}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title="Delete Group"
        description={
          confirmTarget
            ? `Delete "${confirmTarget.name}"? ${
                confirmTarget.node_count > 0
                  ? `${confirmTarget.node_count} node(s) will be moved out of this group (they stay registered).`
                  : "This group has no nodes."
              }`
            : ""
        }
        confirmLabel="Delete"
        destructive
        onConfirm={handleConfirmDelete}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Group</DialogTitle>
            <DialogDescription>
              Add a new node group to organize your machines.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="group-name">Name</Label>
              <Input
                id="group-name"
                placeholder="production-cluster"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="group-desc">Description</Label>
              <Input
                id="group-desc"
                placeholder="Production Kubernetes cluster nodes"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!name.trim()}>
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
