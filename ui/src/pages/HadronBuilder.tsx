import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { getArtifact } from "@/api/artifacts";
import {
  createHadronArtifact,
  listHadronBaseVersions,
  listHadronFirmware,
  listHadronLayers,
  type HadronFirmwareItem,
  type HadronLayerItem,
} from "@/api/hadron";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Plus, Trash2, ArrowUp, ArrowDown, Loader2 } from "lucide-react";
import { toast } from "@/hooks/useToast";

// Platforms are constrained to the buildx values the backend accepts, so an
// operator can't smuggle in linux/riscv64 or something else that would fail
// validation on POST.
const PLATFORMS = ["linux/amd64", "linux/arm64"] as const;

// Baseline base-image tag we fall back to when the catalog endpoint is empty
// (e.g. rate-limited on first load) — matches the README's default so a fresh
// wizard is never a dead-end.
const DEFAULT_BASE = "ghcr.io/kairos-io/hadron:main";

export function HadronBuilder() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [name, setName] = useState("");
  const [baseVersion, setBaseVersion] = useState<string>("main");
  const [baseCustom, setBaseCustom] = useState<string>("");
  const [baseTags, setBaseTags] = useState<string[]>([]);
  const [firmwareCatalog, setFirmwareCatalog] = useState<HadronFirmwareItem[]>([]);
  const [layersCatalog, setLayersCatalog] = useState<HadronLayerItem[]>([]);
  const [firmware, setFirmware] = useState<string[]>([]);
  const [layers, setLayers] = useState<string[]>([]);
  const [extraDockerfile, setExtraDockerfile] = useState("");
  const [platforms, setPlatforms] = useState<string[]>(["linux/amd64"]);
  // Sensible default: any local tag works for tarball-only builds. Push mode
  // requires the operator to point at a real registry, so we still let them
  // overwrite. Not requiring input up front keeps the Start-build button
  // enabled for the common case (quick local tarball).
  const [outputRef, setOutputRef] = useState("local/hadron:latest");
  const [push, setPush] = useState(false);
  const [produceTarball, setProduceTarball] = useState(true);
  const [noCache, setNoCache] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [freeFirmwareRef, setFreeFirmwareRef] = useState("");
  const [freeLayerRef, setFreeLayerRef] = useState("");
  const [firmwareQuery, setFirmwareQuery] = useState("");
  const [layerQuery, setLayerQuery] = useState("");

  // Clone-hadron pre-fill: navigating from an existing hadron artifact planted
  // the source id in the query string; deserialize its hadronSpec JSON and
  // seed every field so the operator only has to tweak whatever changed.
  useEffect(() => {
    const cloneId = searchParams.get("cloneHadron");
    if (!cloneId) return;
    getArtifact(cloneId)
      .then((a) => {
        if (a.kind !== "hadron" || !a.hadronSpec) return;
        try {
          const spec = JSON.parse(a.hadronSpec) as {
            baseImage?: string;
            firmware?: string[];
            layers?: string[];
            extraDockerfile?: string;
            platforms?: string[];
            outputRef?: string;
            push?: boolean;
            produceTarball?: boolean;
            noCache?: boolean;
          };
          setName(a.name ? `Copy of ${a.name}` : "");
          if (spec.baseImage) setBaseCustom(spec.baseImage);
          setFirmware(spec.firmware ?? []);
          setLayers(spec.layers ?? []);
          setExtraDockerfile(spec.extraDockerfile ?? "");
          setPlatforms(spec.platforms ?? ["linux/amd64"]);
          setOutputRef(spec.outputRef ?? "local/hadron:latest");
          setPush(!!spec.push);
          setProduceTarball(spec.produceTarball !== false);
          setNoCache(!!spec.noCache);
        } catch {
          // Malformed spec on the source row — leave the wizard in its
          // default state so the user can rebuild from scratch.
        }
      })
      .catch(() => {});
  }, [searchParams]);

  useEffect(() => {
    listHadronBaseVersions()
      .then((tags) => {
        setBaseTags(tags);
        // Prefer the first tag as the initial selection so users see the
        // newest release rather than a hard-coded fallback whenever possible.
        if (tags.length > 0) setBaseVersion(tags[0]);
      })
      .catch(() => setBaseTags([]));
    listHadronFirmware().then(setFirmwareCatalog).catch(() => setFirmwareCatalog([]));
    listHadronLayers().then(setLayersCatalog).catch(() => setLayersCatalog([]));
  }, []);

  // Resolved base image ref: the custom text box wins when set so operators
  // can point at private mirrors without waiting for a catalog refresh.
  const resolvedBase = useMemo(() => {
    if (baseCustom.trim()) return baseCustom.trim();
    if (baseVersion) return `ghcr.io/kairos-io/hadron:${baseVersion}`;
    return DEFAULT_BASE;
  }, [baseVersion, baseCustom]);

  // Deterministic Dockerfile preview mirrors the backend renderer so what the
  // user sees on Review is byte-identical to what buildx will consume.
  const dockerfilePreview = useMemo(() => {
    let out = "# Generated by AuroraBoot — do not edit\n";
    out += `FROM ${resolvedBase}\n`;
    // Mirrors pkg/hadron.RenderDockerfile: runtime-detected fix that turns
    // /usr/local/lib/firmware into a real directory (sysext + persistent
    // overlay target) with /lib/firmware pointing to it. No-op on hadron
    // images that already ship the correct layout; unconditionally emitted
    // so the wizard preview matches the backend for any tag/digest/branch.
    out +=
      "RUN if [ -L /usr/local/lib/firmware ] || [ ! -d /usr/local/lib/firmware ]; then \\\n" +
      "        rm -rf /usr/local/lib/firmware /lib/firmware && \\\n" +
      "        mkdir -p /usr/local/lib/firmware && \\\n" +
      "        ln -s /usr/local/lib/firmware /lib/firmware; \\\n" +
      "    fi && \\\n" +
      "    if [ -L /usr/sbin ]; then \\\n" +
      "        rm /usr/sbin && mkdir /usr/sbin; \\\n" +
      "    fi\n";
    for (const ref of firmware) out += `COPY --from=${ref} / /\n`;
    for (const ref of layers) out += `COPY --from=${ref} / /\n`;
    const extra = extraDockerfile.trim();
    if (extra) out += extra + (extra.endsWith("\n") ? "" : "\n");
    return out;
  }, [resolvedBase, firmware, layers, extraDockerfile]);

  function toggleFirmware(image: string, version: string) {
    const ref = `${image}:${version}`;
    setFirmware((cur) => (cur.includes(ref) ? cur.filter((r) => r !== ref) : [...cur, ref]));
  }

  function addFreeFirmware() {
    const ref = freeFirmwareRef.trim();
    if (!ref) return;
    if (!firmware.includes(ref)) setFirmware([...firmware, ref]);
    setFreeFirmwareRef("");
  }

  function addFreeLayer() {
    const ref = freeLayerRef.trim();
    if (!ref) return;
    if (!layers.includes(ref)) setLayers([...layers, ref]);
    setFreeLayerRef("");
  }

  function moveLayer(idx: number, dir: -1 | 1) {
    const next = idx + dir;
    if (next < 0 || next >= layers.length) return;
    const copy = layers.slice();
    [copy[idx], copy[next]] = [copy[next], copy[idx]];
    setLayers(copy);
  }

  function togglePlatform(p: string) {
    setPlatforms((cur) => (cur.includes(p) ? cur.filter((x) => x !== p) : [...cur, p]));
  }

  const submitDisabled =
    submitting ||
    !resolvedBase ||
    platforms.length === 0 ||
    (!push && !produceTarball) ||
    !outputRef.trim();

  async function handleSubmit() {
    if (submitDisabled) return;
    setSubmitting(true);
    try {
      const artifact = await createHadronArtifact(name || "", {
        baseImage: resolvedBase,
        firmware,
        layers,
        extraDockerfile,
        platforms,
        outputRef: outputRef.trim(),
        push,
        produceTarball,
        noCache,
      });
      toast("Hadron build started", "success");
      navigate(`/artifacts/${artifact.id}`);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast(`Failed to start build: ${msg}`, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div>
      {/* Sticky so the Start-build / Cancel actions stay reachable while the
          user scrolls through the (potentially long) firmware and layer
          pickers. bg + border keep the cards below from bleeding through. */}
      <div className="sticky top-0 z-20 -mx-6 px-6 pt-6 mb-6 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80 border-b">
        <PageHeader title="New Hadron Image" description="Compose a Hadron OS image from base + firmware + layers">
          <Button variant="outline" onClick={() => navigate("/artifacts")}>
            Cancel
          </Button>
          <Button
            className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
            disabled={submitDisabled}
            onClick={handleSubmit}
          >
            {submitting && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Start build
          </Button>
        </PageHeader>
      </div>

      <div className="grid gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Name</CardTitle>
          </CardHeader>
          <CardContent>
            <Input
              placeholder="Optional friendly name (e.g. hadron-lab-v1)"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Base image</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div>
              <Label className="text-xs">Hadron version</Label>
              <Select
                value={baseVersion}
                onValueChange={(v) => {
                  setBaseVersion(v);
                  setBaseCustom("");
                }}
                disabled={baseCustom.trim() !== ""}
              >
                <SelectTrigger className="mt-1">
                  <SelectValue placeholder="Choose a tag" />
                </SelectTrigger>
                <SelectContent>
                  {(baseTags.length === 0 ? ["main"] : baseTags).map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="text-xs">Or custom image ref</Label>
              <Input
                className="mt-1 font-mono text-xs"
                placeholder="e.g. mirror.example.com/kairos/hadron:v0.5.0"
                value={baseCustom}
                onChange={(e) => setBaseCustom(e.target.value)}
              />
            </div>
            <p className="text-xs text-muted-foreground">
              Effective ref: <span className="font-mono">{resolvedBase}</span>
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Firmware</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              Each firmware image is copied into the base at build time. Pick only what your hardware needs — the full linux-firmware tree is large.
            </p>
            {firmware.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {firmware.map((ref) => (
                  <Badge key={ref} variant="secondary" className="font-mono text-[10px]">
                    {ref}
                    <button
                      className="ml-1 text-muted-foreground hover:text-foreground"
                      onClick={() => setFirmware(firmware.filter((r) => r !== ref))}
                    >
                      ×
                    </button>
                  </Badge>
                ))}
              </div>
            )}
            {firmwareCatalog.length > 0 ? (
              <>
                <Input
                  placeholder="Filter firmware… (e.g. amdgpu, rtw, nvidia)"
                  value={firmwareQuery}
                  onChange={(e) => setFirmwareQuery(e.target.value)}
                  className="text-xs"
                />
                {(() => {
                  const q = firmwareQuery.trim().toLowerCase();
                  const visible = q
                    ? firmwareCatalog.filter((f) => f.name.toLowerCase().includes(q))
                    : firmwareCatalog;
                  return (
                    <div className="max-h-64 overflow-y-auto border rounded-md p-2 space-y-1">
                      {visible.length === 0 ? (
                        <p className="text-xs italic text-muted-foreground text-center py-2">
                          No firmware matches "{firmwareQuery}"
                        </p>
                      ) : (
                        visible.map((f) => {
                          const ref = `${f.image}:${f.version}`;
                          const selected = firmware.includes(ref);
                          return (
                            <label
                              key={ref}
                              className={`flex items-center gap-2 text-xs cursor-pointer px-2 py-1 rounded hover:bg-muted/60 ${selected ? "bg-[#EE5007]/10" : ""}`}
                            >
                              <input
                                type="checkbox"
                                checked={selected}
                                onChange={() => toggleFirmware(f.image, f.version)}
                              />
                              <span className="font-mono flex-1 truncate">{f.name}</span>
                              <span className="text-muted-foreground text-[10px]">{f.version}</span>
                            </label>
                          );
                        })
                      )}
                    </div>
                  );
                })()}
              </>
            ) : (
              <p className="text-xs italic text-muted-foreground">
                Firmware catalog unavailable — add refs manually below.
              </p>
            )}
            <div className="flex gap-2">
              <Input
                className="font-mono text-xs"
                placeholder="Custom firmware ref e.g. ghcr.io/kairos-io/hadron-firmware/linux-firmware-amdgpu:20260622"
                value={freeFirmwareRef}
                onChange={(e) => setFreeFirmwareRef(e.target.value)}
              />
              <Button variant="outline" size="sm" onClick={addFreeFirmware}>
                <Plus className="h-4 w-4 mr-1" /> Add
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Software layers</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              Layers are applied in order — later layers overwrite files from earlier ones. Drag with the arrows to reorder.
            </p>
            {layers.length > 0 && (
              <ol className="space-y-1 border rounded-md p-2">
                {layers.map((ref, idx) => (
                  <li key={ref} className="flex items-center gap-2 text-xs">
                    <span className="w-6 text-center text-muted-foreground font-mono">{idx + 1}</span>
                    <span className="font-mono flex-1 truncate">{ref}</span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      onClick={() => moveLayer(idx, -1)}
                      disabled={idx === 0}
                    >
                      <ArrowUp className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      onClick={() => moveLayer(idx, 1)}
                      disabled={idx === layers.length - 1}
                    >
                      <ArrowDown className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      onClick={() => setLayers(layers.filter((_, i) => i !== idx))}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </li>
                ))}
              </ol>
            )}
            {layersCatalog.length > 0 ? (
              <>
                <Input
                  placeholder="Filter layers… (e.g. git, gpg)"
                  value={layerQuery}
                  onChange={(e) => setLayerQuery(e.target.value)}
                  className="text-xs"
                />
                {(() => {
                  const q = layerQuery.trim().toLowerCase();
                  const visible = q
                    ? layersCatalog.filter(
                        (l) =>
                          l.name.toLowerCase().includes(q) ||
                          (l.description || "").toLowerCase().includes(q) ||
                          (l.title || "").toLowerCase().includes(q)
                      )
                    : layersCatalog;
                  return (
                    <div className="max-h-64 overflow-y-auto border rounded-md p-2 space-y-1">
                      {visible.length === 0 ? (
                        <p className="text-xs italic text-muted-foreground text-center py-2">
                          No layer matches "{layerQuery}"
                        </p>
                      ) : (
                        visible.map((l) => {
                          const ref = `${l.image}:${l.latest || (l.tags?.[0] ?? "latest")}`;
                          const selected = layers.includes(ref);
                          return (
                            <label
                              key={l.name}
                              className={`flex items-center gap-2 text-xs cursor-pointer px-2 py-1 rounded hover:bg-muted/60 ${selected ? "bg-[#EE5007]/10" : ""}`}
                            >
                              <input
                                type="checkbox"
                                checked={selected}
                                onChange={() => {
                                  setLayers((cur) =>
                                    cur.includes(ref) ? cur.filter((r) => r !== ref) : [...cur, ref]
                                  );
                                }}
                              />
                              <span className="font-mono min-w-[6ch]">{l.name}</span>
                              <span className="flex-1 text-muted-foreground truncate">
                                {l.description || l.title}
                              </span>
                              <span className="text-muted-foreground text-[10px]">{l.latest}</span>
                            </label>
                          );
                        })
                      )}
                    </div>
                  );
                })()}
              </>
            ) : (
              <p className="text-xs italic text-muted-foreground">
                Layer catalog unavailable — add refs manually below.
              </p>
            )}
            <div className="flex gap-2">
              <Input
                className="font-mono text-xs"
                placeholder="Custom layer ref e.g. ghcr.io/kairos-io/git:2.55.0"
                value={freeLayerRef}
                onChange={(e) => setFreeLayerRef(e.target.value)}
              />
              <Button variant="outline" size="sm" onClick={addFreeLayer}>
                <Plus className="h-4 w-4 mr-1" /> Add
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Advanced</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label className="text-xs">Extra Dockerfile fragment (appended verbatim)</Label>
            <Textarea
              className="font-mono text-xs min-h-[6rem]"
              placeholder="RUN echo 'hello' > /etc/motd"
              value={extraDockerfile}
              onChange={(e) => setExtraDockerfile(e.target.value)}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Output</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <Label className="text-xs">Platforms</Label>
              <div className="flex gap-4 mt-1">
                {PLATFORMS.map((p) => (
                  <label key={p} className="flex items-center gap-2 text-xs">
                    <input
                      type="checkbox"
                      checked={platforms.includes(p)}
                      onChange={() => togglePlatform(p)}
                    />
                    <span className="font-mono">{p}</span>
                  </label>
                ))}
              </div>
              {platforms.length === 0 && (
                <p className="text-xs text-red-600 mt-1">Select at least one platform.</p>
              )}
              {platforms.some((p) => p !== "linux/amd64") && platforms.length > 0 && (
                <p className="text-[11px] text-muted-foreground mt-1">
                  Cross-arch RUN steps execute under qemu-user emulation. AuroraBoot
                  auto-registers binfmt via <span className="font-mono">tonistiigi/binfmt</span>{" "}
                  before each build — needs privileged Docker on the host. If a build
                  fails with <span className="font-mono">exec format error</span>, run{" "}
                  <span className="font-mono">docker run --privileged --rm tonistiigi/binfmt --install all</span>{" "}
                  on the host once and retry.
                </p>
              )}
            </div>
            <div>
              <Label className="text-xs">Output image ref</Label>
              <Input
                className="mt-1 font-mono text-xs"
                placeholder="registry.example.com/team/hadron:v1"
                value={outputRef}
                onChange={(e) => setOutputRef(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={push}
                  onChange={(e) => setPush(e.target.checked)}
                />
                Push to registry (requires credentials configured in Settings)
              </label>
              <label className="flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={produceTarball}
                  onChange={(e) => setProduceTarball(e.target.checked)}
                />
                Download as OCI tarball (hadron.oci.tar)
              </label>
              <label className="flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={noCache}
                  onChange={(e) => setNoCache(e.target.checked)}
                />
                Skip cache (force rebuild all layers)
              </label>
              <p className="text-[11px] text-muted-foreground -mt-1 ml-6">
                Use when re-running with the same output ref but different firmware/layers,
                otherwise buildx may return stale cached layers.
              </p>
              {!push && !produceTarball && (
                <p className="text-xs text-red-600">At least one output destination is required.</p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Dockerfile preview</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="font-mono text-xs bg-muted/40 border rounded-md p-3 overflow-x-auto whitespace-pre">
              {dockerfilePreview}
            </pre>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
