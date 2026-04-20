import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { StatusBadge } from "@/components/StatusBadge";
import { Wifi, Server, Loader2 } from "lucide-react";
import {
  type BMCTarget,
  type NetbootStatus,
  listBMCTargets,
  createBMCTarget,
  deployRedfish,
  getNetbootStatus,
  startNetboot,
  stopNetboot,
} from "@/api/deployments";

interface DeployDialogProps {
  artifactId: string;
  artifactFiles: string[];
  hasNetboot: boolean;
  onClose: () => void;
}

export function DeployDialog({
  artifactId,
  artifactFiles,
  hasNetboot,
  onClose,
}: DeployDialogProps) {
  const hasIso = artifactFiles.some((f) => f.endsWith(".iso"));
  const defaultTab = hasNetboot ? "pxe" : "redfish";

  // PXE state
  const [netbootStatus, setNetbootStatus] = useState<NetbootStatus | null>(null);
  const [pxeLoading, setPxeLoading] = useState(false);

  // RedFish state
  const [bmcTargets, setBmcTargets] = useState<BMCTarget[]>([]);
  const [selectedTarget, setSelectedTarget] = useState<string>("");
  const [deploying, setDeploying] = useState(false);
  const [deployError, setDeployError] = useState("");
  const [deploySuccess, setDeploySuccess] = useState(false);

  // New BMC target form
  const [showNewTarget, setShowNewTarget] = useState(false);
  const [newTarget, setNewTarget] = useState({
    name: "",
    endpoint: "",
    vendor: "dell",
    username: "",
    password: "",
    verifySSL: false,
  });
  const [creatingTarget, setCreatingTarget] = useState(false);

  useEffect(() => {
    if (hasNetboot) {
      getNetbootStatus().then(setNetbootStatus).catch(() => {});
    }
    if (hasIso) {
      listBMCTargets().then(setBmcTargets).catch(() => {});
    }
  }, [hasNetboot, hasIso]);

  // Poll netboot status while running
  useEffect(() => {
    if (!hasNetboot || !netbootStatus?.running) return;
    const interval = setInterval(() => {
      getNetbootStatus().then(setNetbootStatus).catch(() => {});
    }, 3000);
    return () => clearInterval(interval);
  }, [hasNetboot, netbootStatus?.running]);

  async function handlePxeToggle() {
    setPxeLoading(true);
    try {
      if (netbootStatus?.running) {
        await stopNetboot();
      } else {
        await startNetboot(artifactId);
      }
      const status = await getNetbootStatus();
      setNetbootStatus(status);
    } catch {
      // ignore
    } finally {
      setPxeLoading(false);
    }
  }

  async function handleRedfishDeploy() {
    setDeploying(true);
    setDeployError("");
    setDeploySuccess(false);
    try {
      await deployRedfish(artifactId, { bmcTargetId: selectedTarget });
      setDeploySuccess(true);
    } catch (err) {
      setDeployError(err instanceof Error ? err.message : "Deploy failed");
    } finally {
      setDeploying(false);
    }
  }

  async function handleCreateTarget(e: React.FormEvent) {
    e.preventDefault();
    setCreatingTarget(true);
    try {
      const created = await createBMCTarget(newTarget);
      setBmcTargets((prev) => [...prev, created]);
      setSelectedTarget(created.id);
      setShowNewTarget(false);
      setNewTarget({ name: "", endpoint: "", vendor: "dell", username: "", password: "", verifySSL: false });
    } catch {
      // ignore
    } finally {
      setCreatingTarget(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Deploy Artifact</DialogTitle>
          <DialogDescription>
            Deploy this artifact to bare-metal nodes via PXE boot or RedFish BMC.
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue={defaultTab}>
          <TabsList className="w-full">
            {hasNetboot && (
              <TabsTrigger value="pxe" className="flex-1 gap-2">
                <Wifi className="h-4 w-4" /> PXE Boot
              </TabsTrigger>
            )}
            {hasIso && (
              <TabsTrigger value="redfish" className="flex-1 gap-2">
                <Server className="h-4 w-4" /> RedFish
              </TabsTrigger>
            )}
          </TabsList>

          {hasNetboot && (
            <TabsContent value="pxe" className="space-y-4">
              <div className="rounded-md border p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Netboot Server</span>
                  <StatusBadge status={netbootStatus?.running ? "Running" : "Stopped"} />
                </div>
                {netbootStatus?.running && (
                  <div className="text-xs text-muted-foreground space-y-1">
                    <p>Address: <span className="font-mono">{netbootStatus.address}:{netbootStatus.port}</span></p>
                    <p>Artifact: <span className="font-mono">{netbootStatus.artifactId.slice(0, 12)}</span></p>
                  </div>
                )}
                <Button
                  className="w-full"
                  variant={netbootStatus?.running ? "destructive" : "default"}
                  onClick={handlePxeToggle}
                  disabled={pxeLoading}
                >
                  {pxeLoading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
                  {netbootStatus?.running ? "Stop Netboot" : "Start Netboot"}
                </Button>
              </div>
            </TabsContent>
          )}

          {hasIso && (
            <TabsContent value="redfish" className="space-y-4">
              {/* Target selector */}
              <div className="space-y-2">
                <Label>BMC Target</Label>
                <Select value={selectedTarget} onValueChange={setSelectedTarget}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select a BMC target..." />
                  </SelectTrigger>
                  <SelectContent>
                    {bmcTargets.map((t) => (
                      <SelectItem key={t.id} value={t.id}>
                        {t.name} ({t.endpoint})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-xs text-[#EE5007]"
                  onClick={() => setShowNewTarget(!showNewTarget)}
                >
                  {showNewTarget ? "Cancel" : "+ Add new target"}
                </Button>
              </div>

              {/* New target form */}
              {showNewTarget && (
                <form onSubmit={handleCreateTarget} className="space-y-3 rounded-md border p-4">
                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1">
                      <Label className="text-xs">Name</Label>
                      <Input
                        value={newTarget.name}
                        onChange={(e) => setNewTarget({ ...newTarget, name: e.target.value })}
                        placeholder="my-server"
                        required
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className="text-xs">Endpoint</Label>
                      <Input
                        value={newTarget.endpoint}
                        onChange={(e) => setNewTarget({ ...newTarget, endpoint: e.target.value })}
                        placeholder="https://10.0.0.1"
                        required
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className="text-xs">Username</Label>
                      <Input
                        value={newTarget.username}
                        onChange={(e) => setNewTarget({ ...newTarget, username: e.target.value })}
                        required
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className="text-xs">Password</Label>
                      <Input
                        type="password"
                        value={newTarget.password}
                        onChange={(e) => setNewTarget({ ...newTarget, password: e.target.value })}
                        required
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className="text-xs">Vendor</Label>
                      <Select
                        value={newTarget.vendor}
                        onValueChange={(v) => setNewTarget({ ...newTarget, vendor: v })}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="dell">Dell</SelectItem>
                          <SelectItem value="hp">HP</SelectItem>
                          <SelectItem value="supermicro">Supermicro</SelectItem>
                          <SelectItem value="lenovo">Lenovo</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <Button type="submit" size="sm" disabled={creatingTarget} className="w-full">
                    {creatingTarget && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
                    Add Target
                  </Button>
                </form>
              )}

              {/* Deploy button */}
              {deployError && (
                <div className="bg-red-500/10 border border-red-500/25 text-red-700 rounded-md p-3 text-sm">
                  {deployError}
                </div>
              )}
              {deploySuccess && (
                <div className="bg-green-500/10 border border-green-500/25 text-green-700 rounded-md p-3 text-sm">
                  Deployment started successfully.
                </div>
              )}
              <Button
                className="w-full"
                onClick={handleRedfishDeploy}
                disabled={!selectedTarget || deploying}
              >
                {deploying && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
                Deploy via RedFish
              </Button>
            </TabsContent>
          )}
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
