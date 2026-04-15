import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
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
import { Textarea } from "@/components/ui/textarea";
import { listArtifacts, Artifact } from "@/api/artifacts";
import {
  ArrowUpCircle,
  RotateCcw,
  RefreshCcw,
  FileText,
  Power,
  Terminal,
  ChevronLeft,
  type LucideIcon,
} from "lucide-react";

// CommandDialog runs a two-step wizard: first the user picks an action
// from a grid of cards (no dropdown), then they configure the arguments
// for that specific action. When opened from a quick-action menu the
// parent passes defaultCommand and we skip straight to step 2.

type CommandKey =
  | "upgrade"
  | "upgrade-recovery"
  | "reset"
  | "apply-config"
  | "reboot"
  | "exec";

interface CommandDef {
  key: CommandKey;
  label: string;
  // What the user will see on the primary button ("Send upgrade", etc).
  verb: string;
  description: string;
  icon: LucideIcon;
  // Visual accent for the icon chip on the action card.
  tone: "orange" | "blue" | "amber" | "red";
}

const COMMANDS: CommandDef[] = [
  {
    key: "upgrade",
    label: "Upgrade",
    verb: "Send upgrade",
    description:
      "Upgrade the node's active partition to a new image or artifact.",
    icon: ArrowUpCircle,
    tone: "orange",
  },
  {
    key: "upgrade-recovery",
    label: "Upgrade recovery",
    verb: "Send recovery upgrade",
    description:
      "Replace the recovery partition — active partition stays untouched.",
    icon: RotateCcw,
    tone: "blue",
  },
  {
    key: "apply-config",
    label: "Apply config",
    verb: "Send config",
    description:
      "Push a cloud-config YAML to /oem. Takes effect on next boot.",
    icon: FileText,
    tone: "blue",
  },
  {
    key: "exec",
    label: "Run command",
    verb: "Run command",
    description: "Execute a shell command on the node and capture its output.",
    icon: Terminal,
    tone: "blue",
  },
  {
    key: "reboot",
    label: "Reboot",
    verb: "Send reboot",
    description: "Restart the node after a short delay.",
    icon: Power,
    tone: "amber",
  },
  {
    key: "reset",
    label: "Reset",
    verb: "Send reset",
    description:
      "Factory reset — wipe state partitions and re-run first-boot provisioning.",
    icon: RefreshCcw,
    tone: "red",
  },
];

// Mapped once so the render loop stays tidy. Tones map to the icon chip
// background and the action-card hover affordance.
const TONE_CLASSES: Record<CommandDef["tone"], { chip: string; hover: string }> = {
  orange: {
    chip: "bg-[#EE5007]/10 text-[#EE5007]",
    hover: "hover:border-[#EE5007]/60 hover:bg-[#EE5007]/5",
  },
  blue: {
    chip: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
    hover: "hover:border-sky-500/60 hover:bg-sky-500/5",
  },
  amber: {
    chip: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
    hover: "hover:border-amber-500/60 hover:bg-amber-500/5",
  },
  red: {
    chip: "bg-red-500/10 text-red-600 dark:text-red-400",
    hover: "hover:border-red-500/60 hover:bg-red-500/5",
  },
};

type UpgradeSourceMode = "image" | "artifact";

interface CommandDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (command: string, args: Record<string, unknown>) => void;
  title?: string;
  // If set, opens the dialog straight on the configure step with this
  // command preselected. Used by GroupDetail/NodeDetail quick-action
  // menus so the user doesn't re-pick something they just clicked.
  defaultCommand?: CommandKey | null;
}

export function CommandDialog({
  open,
  onOpenChange,
  onSubmit,
  title = "Send command",
  defaultCommand,
}: CommandDialogProps) {
  const [command, setCommand] = useState<CommandKey | "">("");
  const [imageArg, setImageArg] = useState("");
  const [configArg, setConfigArg] = useState("");
  const [shellCmd, setShellCmd] = useState("");
  const [upgradeSourceMode, setUpgradeSourceMode] =
    useState<UpgradeSourceMode>("image");
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [selectedArtifactId, setSelectedArtifactId] = useState("");
  const [resetOem, setResetOem] = useState(false);
  const [resetPersistent, setResetPersistent] = useState(false);
  const [resetConfig, setResetConfig] = useState("");

  const isUpgrade = command === "upgrade" || command === "upgrade-recovery";
  const activeCommand = COMMANDS.find((c) => c.key === command);

  // Prefill or reset command selection on open/defaultCommand change. When
  // the dialog closes, clear the selection so next open starts clean.
  useEffect(() => {
    if (open) {
      setCommand(defaultCommand ?? "");
    } else {
      setCommand("");
      setImageArg("");
      setConfigArg("");
      setShellCmd("");
      setUpgradeSourceMode("image");
      setSelectedArtifactId("");
      setResetOem(false);
      setResetPersistent(false);
      setResetConfig("");
    }
  }, [open, defaultCommand]);

  useEffect(() => {
    if (open && isUpgrade && upgradeSourceMode === "artifact") {
      listArtifacts()
        .then((all) =>
          setArtifacts(
            all.filter((a) => a.phase === "Ready" && !!a.containerImage),
          ),
        )
        .catch(() => setArtifacts([]));
    }
  }, [open, isUpgrade, upgradeSourceMode]);

  function handleSubmit() {
    if (!command) return;
    const args: Record<string, unknown> = {};

    if (isUpgrade) {
      if (upgradeSourceMode === "artifact") {
        args.source = "artifact:" + selectedArtifactId;
      } else if (imageArg) {
        args.source = "oci:" + imageArg;
      }
      if (command === "upgrade-recovery") {
        args.recovery = "true";
      }
    }

    if (command === "reset") {
      if (resetOem) args["reset-oem"] = "true";
      if (resetPersistent) args["reset-persistent"] = "true";
      if (resetConfig.trim()) args.config = resetConfig.trim();
    }

    if (command === "apply-config") {
      if (configArg.trim()) args.config = configArg.trim();
    }

    if (shellCmd) args.command = shellCmd;

    onSubmit(command, args);
    onOpenChange(false);
  }

  // Whether the Send button should be enabled given the current args.
  const canSubmit = (() => {
    if (!command) return false;
    if (command === "exec") return shellCmd.trim().length > 0;
    if (command === "upgrade" || command === "upgrade-recovery") {
      if (upgradeSourceMode === "image") return imageArg.trim().length > 0;
      return selectedArtifactId !== "";
    }
    return true;
  })();

  const onPickStep = !command;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-2">
            {!onPickStep && (
              <button
                type="button"
                onClick={() => setCommand("")}
                className="inline-flex items-center text-xs text-muted-foreground hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded"
                title="Pick a different action"
              >
                <ChevronLeft className="h-3.5 w-3.5 mr-0.5" />
                Back
              </button>
            )}
            <DialogTitle className="flex-1 truncate">
              {onPickStep
                ? title
                : `${activeCommand?.label ?? ""}`}
            </DialogTitle>
          </div>
          {onPickStep && (
            <DialogDescription>
              Pick an action. You'll configure its details on the next step.
            </DialogDescription>
          )}
        </DialogHeader>

        {/* STEP 1 — pick an action */}
        {onPickStep && (
          <div className="grid grid-cols-2 gap-3 py-2">
            {COMMANDS.map((c) => {
              const Icon = c.icon;
              const tone = TONE_CLASSES[c.tone];
              return (
                <button
                  key={c.key}
                  type="button"
                  onClick={() => setCommand(c.key)}
                  className={
                    "group flex flex-col items-start gap-2 rounded-lg border border-border p-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 " +
                    tone.hover
                  }
                >
                  <div className={`flex h-8 w-8 items-center justify-center rounded-md ${tone.chip}`}>
                    <Icon className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-sm font-semibold leading-tight">{c.label}</div>
                    <p className="text-xs text-muted-foreground mt-1 leading-snug">
                      {c.description}
                    </p>
                  </div>
                </button>
              );
            })}
          </div>
        )}

        {/* STEP 2 — configure the picked action */}
        {!onPickStep && activeCommand && (
          <div className="grid gap-4 py-2">
            {/* Active action summary row */}
            <div className="flex items-start gap-3 rounded-lg border bg-muted/30 px-3 py-2.5">
              <div
                className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md ${TONE_CLASSES[activeCommand.tone].chip}`}
              >
                <activeCommand.icon className="h-4 w-4" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-semibold">{activeCommand.label}</div>
                <p className="text-xs text-muted-foreground">
                  {activeCommand.description}
                </p>
              </div>
            </div>

            {isUpgrade && (
              <>
                <div className="grid gap-2">
                  <Label>Upgrade source</Label>
                  <Select
                    value={upgradeSourceMode}
                    onValueChange={(v) => setUpgradeSourceMode(v as UpgradeSourceMode)}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="image">From image reference</SelectItem>
                      <SelectItem value="artifact">From built artifact</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {upgradeSourceMode === "image" && (
                  <div className="grid gap-2">
                    <Label>Image</Label>
                    <Input
                      placeholder="e.g. quay.io/kairos/kairos:latest"
                      value={imageArg}
                      onChange={(e) => setImageArg(e.target.value)}
                    />
                  </div>
                )}

                {upgradeSourceMode === "artifact" && (
                  <div className="grid gap-2">
                    <Label>Artifact</Label>
                    {artifacts.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        No ready artifacts with a container image available.
                      </p>
                    ) : (
                      <Select
                        value={selectedArtifactId}
                        onValueChange={setSelectedArtifactId}
                      >
                        <SelectTrigger className="max-w-full">
                          <SelectValue placeholder="Select artifact..." />
                        </SelectTrigger>
                        <SelectContent className="max-w-[min(28rem,90vw)]">
                          {artifacts.filter((a) => a.saved).length > 0 && (
                            <>
                              <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                                Saved
                              </div>
                              {artifacts
                                .filter((a) => a.saved)
                                .map((a) => (
                                  <SelectItem key={a.id} value={a.id}>
                                    <div className="flex flex-col gap-0.5 min-w-0">
                                      <span className="truncate font-medium">
                                        {a.name || a.baseImage?.split("/").pop() || "Unnamed"}
                                      </span>
                                      <span className="text-xs text-muted-foreground truncate">
                                        {a.id.slice(0, 8)}
                                      </span>
                                    </div>
                                  </SelectItem>
                                ))}
                              <div className="-mx-1 my-1 h-px bg-muted" />
                            </>
                          )}
                          {artifacts
                            .filter((a) => !a.saved)
                            .map((a) => (
                              <SelectItem key={a.id} value={a.id}>
                                <div className="flex flex-col gap-0.5 min-w-0">
                                  <span className="truncate">
                                    {a.name || a.baseImage?.split("/").pop() || "Unnamed"}
                                  </span>
                                  <span className="text-xs text-muted-foreground truncate">
                                    {a.id.slice(0, 8)}
                                  </span>
                                </div>
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                    )}
                  </div>
                )}
              </>
            )}

            {command === "reset" && (
              <>
                <div className="grid gap-3">
                  <Label>Reset options</Label>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={resetOem}
                      onChange={(e) => setResetOem(e.target.checked)}
                      className="rounded border-input"
                    />
                    Reset OEM partition
                  </label>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={resetPersistent}
                      onChange={(e) => setResetPersistent(e.target.checked)}
                      className="rounded border-input"
                    />
                    Reset persistent data
                  </label>
                </div>
                <div className="grid gap-2">
                  <Label>Cloud config to apply after reset (optional)</Label>
                  <Textarea
                    placeholder={"#cloud-config\ninstall:\n  auto: true"}
                    value={resetConfig}
                    onChange={(e) => setResetConfig(e.target.value)}
                    rows={5}
                    className="font-mono text-sm"
                  />
                  <p className="text-xs text-muted-foreground">
                    Written to /oem after reset completes. If OEM is wiped, this becomes the only config.
                  </p>
                </div>
              </>
            )}

            {command === "apply-config" && (
              <div className="grid gap-2">
                <Label>Cloud configuration (YAML)</Label>
                <Textarea
                  placeholder={"#cloud-config\nstages:\n  boot:\n    - name: example"}
                  value={configArg}
                  onChange={(e) => setConfigArg(e.target.value)}
                  rows={6}
                  className="font-mono text-sm"
                />
                <p className="text-xs text-muted-foreground">
                  Written to /oem/99_auroraboot_remote.yaml. Reboot the node to apply.
                </p>
              </div>
            )}

            {command === "reboot" && (
              <div className="rounded-md border border-amber-400/50 bg-amber-50 px-3 py-2.5 text-sm text-amber-900 dark:border-amber-400/40 dark:bg-amber-950/60 dark:text-amber-100">
                The node will restart after a 10-second delay.
              </div>
            )}

            {command === "exec" && (
              <div className="grid gap-2">
                <Label>Shell command</Label>
                <Input
                  placeholder="e.g. uname -a"
                  value={shellCmd}
                  onChange={(e) => setShellCmd(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Output will be captured in the command history on the node page.
                </p>
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          {!onPickStep && (
            <Button
              className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
              onClick={handleSubmit}
              disabled={!canSubmit}
            >
              {activeCommand?.verb ?? "Send"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
