import { Fragment, useEffect, useState } from "react";
import { PageHeader } from "@/components/PageHeader";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Cpu,
  MemoryStick,
  Loader2,
  Pencil,
  Trash2,
  Search,
  Plus,
  ServerCog,
} from "lucide-react";
import {
  type BMCTarget,
  type InspectResult,
  listBMCTargets,
  createBMCTarget,
  updateBMCTarget,
  deleteBMCTarget,
  inspectHardware,
} from "@/api/deployments";
import {
  type ImageSourceSettings,
  getImageSourceSettings,
  updateImageSourceSettings,
} from "@/api/settings";
import { toast } from "@/hooks/useToast";

// Vendor options match the inline "new target" form in DeployDialog so a target
// created here behaves identically to one created mid-deploy.
const VENDORS = [
  { value: "dell", label: "Dell" },
  { value: "hp", label: "HP" },
  { value: "supermicro", label: "Supermicro" },
  { value: "lenovo", label: "Lenovo" },
];

type FormState = {
  name: string;
  endpoint: string;
  vendor: string;
  username: string;
  password: string;
  verifySSL: boolean;
  systemId: string;
  imageUrl: string;
};

const EMPTY_FORM: FormState = {
  name: "",
  endpoint: "",
  vendor: "dell",
  username: "",
  password: "",
  verifySSL: false,
  systemId: "",
  imageUrl: "",
};

function vendorLabel(vendor: string): string {
  return VENDORS.find((v) => v.value === vendor)?.label ?? vendor;
}

// imageSourceBadge describes how a BMC row resolves its image source, given the
// per-BMC override and the global image-source settings.
function imageSourceBadge(
  target: BMCTarget,
  settings: ImageSourceSettings | null
): { label: string; variant: "default" | "secondary" | "outline" } {
  if (target.imageUrl) {
    return { label: "Per-BMC URL", variant: "default" };
  }
  if (settings?.defaultImageURL) {
    return { label: "Global default", variant: "secondary" };
  }
  if (settings?.localServe.configured && settings.localServe.enabled) {
    return { label: "AuroraBoot-served", variant: "outline" };
  }
  return { label: "Unset", variant: "outline" };
}

export function BMCRegistration() {
  const [targets, setTargets] = useState<BMCTarget[]>([]);
  const [loading, setLoading] = useState(true);

  // Global image-source settings (model b): the top-of-page panel. `imgSource`
  // is null until the first fetch resolves.
  const [imgSource, setImgSource] = useState<ImageSourceSettings | null>(null);
  const [imgDefaultURL, setImgDefaultURL] = useState("");
  const [imgAdvertisedURL, setImgAdvertisedURL] = useState("");
  const [savingImgSource, setSavingImgSource] = useState(false);

  // Add/edit modal. `editing` holds the target being edited, or null for "add".
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BMCTarget | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);

  // Delete confirmation.
  const [deleteTarget, setDeleteTarget] = useState<BMCTarget | null>(null);

  // Per-row inspect state, keyed by target id. Inspect is on-demand only.
  const [inspectingId, setInspectingId] = useState<string | null>(null);
  const [inspectResults, setInspectResults] = useState<
    Record<string, InspectResult>
  >({});
  const [inspectErrors, setInspectErrors] = useState<Record<string, string>>({});

  function fetchTargets() {
    listBMCTargets()
      .then(setTargets)
      .catch((err) =>
        toast(`Failed to load BMC targets: ${(err as Error).message}`, "error")
      )
      .finally(() => setLoading(false));
  }

  function applyImgSource(s: ImageSourceSettings) {
    setImgSource(s);
    setImgDefaultURL(s.defaultImageURL);
    setImgAdvertisedURL(s.localServe.advertisedURL);
  }

  function fetchImageSource() {
    getImageSourceSettings()
      .then(applyImgSource)
      .catch((err) =>
        toast(
          `Failed to load image-source settings: ${(err as Error).message}`,
          "error"
        )
      );
  }

  useEffect(() => {
    fetchTargets();
    fetchImageSource();
  }, []);

  async function handleSaveImageSource(e: React.FormEvent) {
    e.preventDefault();
    setSavingImgSource(true);
    try {
      const updated = await updateImageSourceSettings({
        defaultImageURL: imgDefaultURL,
        // advertisedURL is only meaningful when a listener is configured.
        ...(imgSource?.localServe.configured
          ? { localServeAdvertisedURL: imgAdvertisedURL }
          : {}),
      });
      applyImgSource(updated);
      toast("Saved image source settings", "success");
    } catch (err) {
      toast(
        `Failed to save image-source settings: ${(err as Error).message}`,
        "error"
      );
    } finally {
      setSavingImgSource(false);
    }
  }

  async function handleToggleLocalServe(enabled: boolean) {
    try {
      const updated = await updateImageSourceSettings({
        localServeEnabled: enabled,
      });
      applyImgSource(updated);
    } catch (err) {
      toast(
        `Failed to update local serving: ${(err as Error).message}`,
        "error"
      );
    }
  }

  function openAdd() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setFormOpen(true);
  }

  function openEdit(t: BMCTarget) {
    setEditing(t);
    // Password is never returned by the API; leave it blank to keep existing.
    setForm({
      name: t.name,
      endpoint: t.endpoint,
      vendor: t.vendor,
      username: t.username,
      password: "",
      verifySSL: t.verifySSL,
      systemId: t.systemId ?? "",
      imageUrl: t.imageUrl ?? "",
    });
    setFormOpen(true);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (editing) {
        const payload: FormState = { ...form };
        // Omit a blank password so the backend keeps the stored credential.
        const updated = await updateBMCTarget(
          editing.id,
          payload.password
            ? payload
            : {
                name: payload.name,
                endpoint: payload.endpoint,
                vendor: payload.vendor,
                username: payload.username,
                verifySSL: payload.verifySSL,
                systemId: payload.systemId,
                imageUrl: payload.imageUrl,
              }
        );
        setTargets((prev) =>
          prev.map((t) => (t.id === updated.id ? updated : t))
        );
        toast(`Updated BMC "${updated.name}"`, "success");
      } else {
        const created = await createBMCTarget(form);
        setTargets((prev) => [...prev, created]);
        toast(`Added BMC "${created.name}"`, "success");
      }
      setFormOpen(false);
    } catch (err) {
      toast(
        `Failed to save BMC: ${(err as Error).message}`,
        "error"
      );
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    const t = deleteTarget;
    try {
      await deleteBMCTarget(t.id);
      setTargets((prev) => prev.filter((x) => x.id !== t.id));
      // Drop any inspect state we held for this target.
      setInspectResults((prev) => {
        const next = { ...prev };
        delete next[t.id];
        return next;
      });
      setInspectErrors((prev) => {
        const next = { ...prev };
        delete next[t.id];
        return next;
      });
      toast(`Deleted BMC "${t.name}"`, "success");
    } catch (err) {
      toast(`Failed to delete BMC: ${(err as Error).message}`, "error");
    }
  }

  async function handleInspect(t: BMCTarget) {
    setInspectingId(t.id);
    setInspectErrors((prev) => {
      const next = { ...prev };
      delete next[t.id];
      return next;
    });
    try {
      const result = await inspectHardware(t.id);
      setInspectResults((prev) => ({ ...prev, [t.id]: result }));
    } catch (err) {
      setInspectErrors((prev) => ({
        ...prev,
        [t.id]: err instanceof Error ? err.message : "Inspection failed",
      }));
    } finally {
      setInspectingId(null);
    }
  }

  return (
    <div>
      <PageHeader
        title="BMC Registration"
        description="Register and manage baseboard management controllers for RedFish deployments"
      >
        <Button
          className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
          onClick={openAdd}
        >
          <Plus className="h-4 w-4 mr-2" />
          Add BMC
        </Button>
      </PageHeader>

      {/* Image source panel (model b global): the default image URL every BMC
          falls back to, plus — when a local-serve listener was configured at
          launch — the toggle and advertised URL for AuroraBoot-served ISOs. */}
      <Card className="mb-6">
        <CardContent className="p-5 space-y-4">
          <div>
            <h2 className="text-sm font-semibold">Image source</h2>
            <p className="text-xs text-muted-foreground mt-0.5">
              Where BMCs pull the install ISO from. A per-BMC override (below)
              takes precedence over the global default; a per-deploy URL takes
              precedence over both.
            </p>
          </div>
          <form onSubmit={handleSaveImageSource} className="space-y-4">
            <div className="space-y-1">
              <Label className="text-xs">Default image URL</Label>
              <Input
                value={imgDefaultURL}
                onChange={(e) => setImgDefaultURL(e.target.value)}
                placeholder="https://images.example.com/kairos.iso"
              />
              <p className="text-xs text-muted-foreground">
                The URL every BMC pulls the ISO from unless it has its own
                override. Leave blank to require a per-BMC or per-deploy URL.
              </p>
            </div>

            {imgSource?.localServe.configured && (
              <div className="rounded-md border p-4 space-y-3">
                <label className="flex items-center gap-2 cursor-pointer text-sm font-medium">
                  <input
                    type="checkbox"
                    checked={imgSource.localServe.enabled}
                    onChange={(e) => handleToggleLocalServe(e.target.checked)}
                  />
                  AuroraBoot serves the built artifact ISO
                </label>
                <p className="text-xs text-muted-foreground">
                  When enabled, deploys with no operator-supplied URL serve the
                  artifact's own ISO over a tokenized, BMC-reachable URL
                  {imgSource.localServe.usesTLS ? " (HTTPS)." : " (HTTP)."}
                </p>
                <div className="space-y-1">
                  <Label className="text-xs">Advertised URL</Label>
                  <Input
                    value={imgAdvertisedURL}
                    onChange={(e) => setImgAdvertisedURL(e.target.value)}
                    placeholder="http://10.0.0.5:8090"
                  />
                  <p className="text-xs text-muted-foreground">
                    The base URL the BMC reaches the served ISO at (the bind
                    address is fixed at launch via --redfish-serve-addr).
                  </p>
                </div>
              </div>
            )}

            {imgSource && !imgSource.localServe.configured && (
              <p className="text-xs text-muted-foreground">
                AuroraBoot-served ISOs are unavailable: no{" "}
                <code className="font-mono">--redfish-serve-addr</code> was set
                at launch. Use a default image URL or per-BMC override instead.
              </p>
            )}

            <div className="flex justify-end">
              <Button type="submit" disabled={savingImgSource}>
                {savingImgSource && (
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                )}
                Save image source
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground">
              Loading...
            </div>
          ) : targets.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
              <ServerCog className="h-12 w-12 mb-3 text-muted-foreground/30" />
              <div className="text-center">
                <p className="font-medium text-foreground">
                  No BMCs registered yet
                </p>
                <p className="text-sm mt-1">
                  Add a baseboard management controller to deploy artifacts over
                  RedFish.
                </p>
              </div>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Endpoint</TableHead>
                  <TableHead>Vendor</TableHead>
                  <TableHead>System ID</TableHead>
                  <TableHead>Image source</TableHead>
                  <TableHead>TLS verify</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {targets.map((t) => {
                  const result = inspectResults[t.id];
                  const error = inspectErrors[t.id];
                  const busy = inspectingId === t.id;
                  return (
                    <Fragment key={t.id}>
                      <TableRow>
                        <TableCell className="font-medium">{t.name}</TableCell>
                        <TableCell className="font-mono text-xs">
                          {t.endpoint}
                        </TableCell>
                        <TableCell>{vendorLabel(t.vendor)}</TableCell>
                        <TableCell>
                          {t.systemId ? (
                            <span className="font-mono text-xs">{t.systemId}</span>
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              auto
                            </span>
                          )}
                        </TableCell>
                        <TableCell>
                          {(() => {
                            const badge = imageSourceBadge(t, imgSource);
                            return (
                              <Badge
                                variant={badge.variant}
                                title={t.imageUrl || undefined}
                              >
                                {badge.label}
                              </Badge>
                            );
                          })()}
                        </TableCell>
                        <TableCell>
                          <Badge variant={t.verifySSL ? "default" : "secondary"}>
                            {t.verifySSL ? "Enabled" : "Disabled"}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="outline"
                              size="sm"
                              className="gap-1.5 text-xs"
                              onClick={() => handleInspect(t)}
                              disabled={busy}
                            >
                              {busy ? (
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              ) : (
                                <Search className="h-3.5 w-3.5" />
                              )}
                              Inspect
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              aria-label={`Edit ${t.name}`}
                              onClick={() => openEdit(t)}
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-destructive hover:text-destructive"
                              aria-label={`Delete ${t.name}`}
                              onClick={() => setDeleteTarget(t)}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>

                      {/* Inspect error row */}
                      {error && (
                        <TableRow>
                          <TableCell colSpan={7} className="pt-0">
                            <div className="bg-red-500/10 border border-red-500/25 text-red-700 rounded-md p-3 text-sm whitespace-pre-wrap">
                              {error}
                            </div>
                          </TableCell>
                        </TableRow>
                      )}

                      {/* Inspect result row — same rendering style as DeployDialog */}
                      {result && (
                        <TableRow>
                          <TableCell colSpan={7} className="pt-0">
                            <div className="rounded-md border p-4 space-y-3">
                              <div className="text-sm font-medium">
                                Hardware inspection
                              </div>
                              <div className="grid grid-cols-2 gap-2 text-xs max-w-md">
                                <div className="text-muted-foreground">Model</div>
                                <div className="font-mono">
                                  {result.model || "-"}
                                </div>
                                <div className="text-muted-foreground">
                                  Manufacturer
                                </div>
                                <div className="font-mono">
                                  {result.manufacturer || "-"}
                                </div>
                                <div className="text-muted-foreground">Serial</div>
                                <div className="font-mono">
                                  {result.serialNumber || "-"}
                                </div>
                                <div className="text-muted-foreground flex items-center gap-1">
                                  <MemoryStick className="h-3.5 w-3.5" /> Memory
                                </div>
                                <div className="font-mono">
                                  {result.memoryGiB} GiB
                                </div>
                                <div className="text-muted-foreground flex items-center gap-1">
                                  <Cpu className="h-3.5 w-3.5" /> Processors
                                </div>
                                <div className="font-mono">
                                  {result.processorCount}
                                </div>
                              </div>
                              {result.supportedFeatures.length > 0 && (
                                <div className="space-y-1">
                                  <div className="text-xs text-muted-foreground">
                                    Supported features
                                  </div>
                                  <div className="flex flex-wrap gap-1">
                                    {result.supportedFeatures.map((f) => (
                                      <span
                                        key={f}
                                        className="rounded bg-secondary px-1.5 py-0.5 text-xs font-mono"
                                      >
                                        {f}
                                      </span>
                                    ))}
                                  </div>
                                </div>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Add / Edit modal */}
      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? "Edit BMC" : "Add BMC"}</DialogTitle>
            <DialogDescription>
              {editing
                ? "Update the connection details for this baseboard management controller."
                : "Register a baseboard management controller for RedFish deployments."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label className="text-xs">Name</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="my-server"
                  required
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Endpoint</Label>
                <Input
                  value={form.endpoint}
                  onChange={(e) =>
                    setForm({ ...form, endpoint: e.target.value })
                  }
                  placeholder="https://10.0.0.1"
                  required
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Vendor</Label>
                <Select
                  value={form.vendor}
                  onValueChange={(v) => setForm({ ...form, vendor: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {VENDORS.map((v) => (
                      <SelectItem key={v.value} value={v.value}>
                        {v.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Username</Label>
                <Input
                  value={form.username}
                  onChange={(e) =>
                    setForm({ ...form, username: e.target.value })
                  }
                  required
                />
              </div>
              <div className="space-y-1 col-span-2">
                <Label className="text-xs">Password</Label>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                  placeholder={
                    editing ? "Leave blank to keep existing" : undefined
                  }
                  // Password is required on create only; on edit a blank value
                  // keeps the stored credential.
                  required={!editing}
                />
                {editing && (
                  <p className="text-xs text-muted-foreground">
                    Leave blank to keep the existing password.
                  </p>
                )}
              </div>
              <div className="space-y-1 col-span-2">
                <Label className="text-xs">System ID</Label>
                <Input
                  value={form.systemId}
                  onChange={(e) =>
                    setForm({ ...form, systemId: e.target.value })
                  }
                  placeholder="optional"
                />
                <p className="text-xs text-muted-foreground">
                  Leave blank for single-system BMCs; required when the BMC
                  exposes multiple ComputerSystems.
                </p>
              </div>
              <div className="space-y-1 col-span-2">
                <Label className="text-xs">Image URL override</Label>
                <Input
                  value={form.imageUrl}
                  onChange={(e) =>
                    setForm({ ...form, imageUrl: e.target.value })
                  }
                  placeholder="https://images.example.com/kairos.iso"
                />
                <p className="text-xs text-muted-foreground">
                  URL the BMC pulls the ISO from; overrides the global default.
                  Leave blank to use the global default image source.
                </p>
              </div>
            </div>
            <label className="flex items-center gap-2 cursor-pointer text-sm">
              <input
                type="checkbox"
                checked={form.verifySSL}
                onChange={(e) =>
                  setForm({ ...form, verifySSL: e.target.checked })
                }
              />
              Verify TLS certificate
            </label>
            <div className="flex justify-end gap-2 pt-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setFormOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
                {editing ? "Save changes" : "Add BMC"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="Delete BMC"
        description={
          deleteTarget
            ? `Delete BMC "${deleteTarget.name}" (${deleteTarget.endpoint})? This cannot be undone.`
            : ""
        }
        confirmLabel="Delete"
        destructive
        onConfirm={handleDelete}
      />
    </div>
  );
}
