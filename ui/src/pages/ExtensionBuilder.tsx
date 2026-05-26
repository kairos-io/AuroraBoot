import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { listArtifacts, type Artifact } from "@/api/artifacts";
import {
  createExtension,
  type CreateExtensionInput,
  type ExtensionType,
} from "@/api/extensions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PageHeader } from "@/components/PageHeader";
import { HierarchyChipInput } from "@/components/HierarchyChipInput";

type SourceMode = "artifact" | "image" | "dockerfile";
type Arch = "amd64" | "arm64" | "riscv64";

export function ExtensionBuilder() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [sourceMode, setSourceMode] = useState<SourceMode>("image");
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [selectedArtifactId, setSelectedArtifactId] = useState("");
  const [extraSteps, setExtraSteps] = useState("");
  const [baseImage, setBaseImage] = useState("");
  const [dockerfile, setDockerfile] = useState("");

  const [type, setType] = useState<ExtensionType>("sysext");
  const [arch, setArch] = useState<Arch>("amd64");
  const [version, setVersion] = useState("v1.0");
  const [signingKeySetId, setSigningKeySetId] = useState("");
  const [hierarchies, setHierarchies] = useState<string[]>([]);
  const [serviceReload, setServiceReload] = useState(false);

  const [submitting, setSubmitting] = useState(false);
  const [submitErr, setSubmitErr] = useState<string | null>(null);

  useEffect(() => {
    listArtifacts()
      .then((rows) => {
        const ready = rows.filter(
          (a) => a.phase === "Ready" && a.containerImage,
        );
        setArtifacts(ready);
        if (ready.length > 0) {
          setSourceMode("artifact");
          setSelectedArtifactId(ready[0].id);
        }
      })
      .catch(() => {});
  }, []);

  async function submit() {
    setSubmitting(true);
    setSubmitErr(null);
    const input: CreateExtensionInput = {
      name,
      type,
      arch,
      version,
      source: {
        mode: sourceMode,
        artifactId:
          sourceMode === "artifact" ? selectedArtifactId : undefined,
        baseImage: sourceMode === "image" ? baseImage : undefined,
        dockerfile: sourceMode === "dockerfile" ? dockerfile : undefined,
        extraSteps:
          sourceMode === "artifact" && extraSteps ? extraSteps : undefined,
      },
      signingKeySetId: signingKeySetId || undefined,
      hierarchies: type === "sysext" && hierarchies.length > 0 ? hierarchies : undefined,
      serviceReload: type === "sysext" ? serviceReload : false,
    };
    try {
      const status = await createExtension(input);
      navigate(`/extensions/${status.id}`);
    } catch (e) {
      setSubmitErr(String(e));
      setSubmitting(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="Build extension"
        description="A sysext extends /usr; a confext extends /etc. Both ship as a single signed .raw."
      />

      <StepIndicator current={step} />

      {step === 0 && (
        <div className="grid gap-6">
          <div className="max-w-md grid gap-1.5">
            <Label htmlFor="ext-name">Name</Label>
            <Input
              id="ext-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. tailscale-agent"
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Image source</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="flex gap-2">
                <ModeButton
                  active={sourceMode === "artifact"}
                  onClick={() => setSourceMode("artifact")}
                >
                  From artifact
                </ModeButton>
                <ModeButton
                  active={sourceMode === "image"}
                  onClick={() => setSourceMode("image")}
                >
                  Base image
                </ModeButton>
                <ModeButton
                  active={sourceMode === "dockerfile"}
                  onClick={() => setSourceMode("dockerfile")}
                >
                  Dockerfile
                </ModeButton>
              </div>

              {sourceMode === "artifact" && (
                <ArtifactPicker
                  artifacts={artifacts}
                  selectedId={selectedArtifactId}
                  onSelect={setSelectedArtifactId}
                  extraSteps={extraSteps}
                  onExtraStepsChange={setExtraSteps}
                />
              )}
              {sourceMode === "image" && (
                <div className="grid gap-1.5">
                  <Label htmlFor="ext-base">Base image</Label>
                  <Input
                    id="ext-base"
                    value={baseImage}
                    onChange={(e) => setBaseImage(e.target.value)}
                    placeholder="e.g. ubuntu:24.04"
                  />
                </div>
              )}
              {sourceMode === "dockerfile" && (
                <div className="grid gap-1.5">
                  <Label htmlFor="ext-df">Dockerfile</Label>
                  <Textarea
                    id="ext-df"
                    rows={8}
                    value={dockerfile}
                    onChange={(e) => setDockerfile(e.target.value)}
                    placeholder="FROM ubuntu:24.04\nRUN apt-get install -y curl"
                    className="font-mono text-sm"
                  />
                </div>
              )}
            </CardContent>
          </Card>

          <div className="flex justify-between">
            <Button variant="outline" onClick={() => navigate("/extensions")}>
              Cancel
            </Button>
            <Button onClick={() => setStep(1)}>Next →</Button>
          </div>
        </div>
      )}

      {step === 1 && (
        <ConfigureStep
          type={type}
          onType={setType}
          arch={arch}
          onArch={setArch}
          version={version}
          onVersion={setVersion}
          signingKeySetId={signingKeySetId}
          onSigningKeySetId={setSigningKeySetId}
          hierarchies={hierarchies}
          onHierarchies={setHierarchies}
          serviceReload={serviceReload}
          onServiceReload={setServiceReload}
          onBack={() => setStep(0)}
          onNext={() => setStep(2)}
        />
      )}

      {step === 2 && (
        <ReviewStep
          name={name}
          type={type}
          arch={arch}
          version={version}
          sourceMode={sourceMode}
          baseImage={baseImage}
          selectedArtifactId={selectedArtifactId}
          dockerfile={dockerfile}
          extraSteps={extraSteps}
          hierarchies={hierarchies}
          serviceReload={serviceReload}
          submitting={submitting}
          submitErr={submitErr}
          onBack={() => setStep(1)}
          onSubmit={submit}
        />
      )}
    </div>
  );
}

function ModeButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant={active ? "default" : "outline"}
      onClick={onClick}
      data-active={active}
    >
      {children}
    </Button>
  );
}

function ArtifactPicker({
  artifacts,
  selectedId,
  onSelect,
  extraSteps,
  onExtraStepsChange,
}: {
  artifacts: Artifact[];
  selectedId: string;
  onSelect: (id: string) => void;
  extraSteps: string;
  onExtraStepsChange: (s: string) => void;
}) {
  if (artifacts.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No Ready artifacts yet — build one first.
      </p>
    );
  }
  const selected = artifacts.find((a) => a.id === selectedId);
  return (
    <div className="grid gap-3">
      <Label>Pick an existing artifact</Label>
      <select
        className="border rounded-md px-3 py-2 text-sm bg-background"
        value={selectedId}
        onChange={(e) => onSelect(e.target.value)}
      >
        {artifacts.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name || a.id} — {a.arch} — {a.kairosVersion}
          </option>
        ))}
      </select>
      {selected && (
        <p className="text-[11px] text-muted-foreground font-mono">
          {selected.containerImage}
        </p>
      )}
      <div className="grid gap-1.5">
        <Label className="text-xs">Steps on top (optional)</Label>
        <Textarea
          rows={4}
          value={extraSteps}
          onChange={(e) => onExtraStepsChange(e.target.value)}
          placeholder="RUN curl -fsSL https://tailscale.com/install.sh | sh"
          className="font-mono text-xs"
        />
        <p className="text-[11px] text-muted-foreground">
          Wrapped in <code>FROM &lt;artifact-image&gt;</code> before the
          extractor runs. Lines starting with <code>FROM</code> are rejected.
        </p>
      </div>
    </div>
  );
}

function ConfigureStep({
  type,
  onType,
  arch,
  onArch,
  version,
  onVersion,
  signingKeySetId,
  onSigningKeySetId,
  hierarchies,
  onHierarchies,
  serviceReload,
  onServiceReload,
  onBack,
  onNext,
}: {
  type: ExtensionType;
  onType: (t: ExtensionType) => void;
  arch: Arch;
  onArch: (a: Arch) => void;
  version: string;
  onVersion: (v: string) => void;
  signingKeySetId: string;
  onSigningKeySetId: (s: string) => void;
  hierarchies: string[];
  onHierarchies: (h: string[]) => void;
  serviceReload: boolean;
  onServiceReload: (s: boolean) => void;
  onBack: () => void;
  onNext: () => void;
}) {
  const required = type && arch && version.trim();
  return (
    <div className="grid gap-6">
      <div className="grid md:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Extension type</CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-2">
            <TypeCard
              label="sysext"
              desc="Overlay on /usr (and additional paths)"
              active={type === "sysext"}
              onClick={() => onType("sysext")}
            />
            <TypeCard
              label="confext"
              desc="Overlay on /etc"
              active={type === "confext"}
              onClick={() => onType("confext")}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Architecture</CardTitle>
          </CardHeader>
          <CardContent className="flex gap-2">
            {(["amd64", "arm64", "riscv64"] as const).map((a) => (
              <Button
                key={a}
                type="button"
                size="sm"
                variant={arch === a ? "default" : "outline"}
                onClick={() => onArch(a)}
              >
                {a}
              </Button>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Version</CardTitle>
          </CardHeader>
          <CardContent>
            <Label htmlFor="ext-version" className="sr-only">
              Version
            </Label>
            <Input
              id="ext-version"
              value={version}
              onChange={(e) => onVersion(e.target.value)}
              placeholder="v1.0"
            />
            <p className="text-[11px] text-muted-foreground mt-1.5">
              Tracked server-side for staleness detection.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Signing (optional)</CardTitle>
          </CardHeader>
          <CardContent>
            <Input
              value={signingKeySetId}
              onChange={(e) => onSigningKeySetId(e.target.value)}
              placeholder="key-set id (optional)"
            />
          </CardContent>
        </Card>
      </div>

      {type === "sysext" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Hierarchies</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4">
            <HierarchyChipInput
              value={hierarchies}
              onChange={onHierarchies}
              implicitRoot="/usr"
              quickAdds={["/opt", "/srv", "/var/lib"]}
            />
            <label className="flex gap-2 items-start text-sm">
              <input
                type="checkbox"
                checked={serviceReload}
                onChange={(e) => onServiceReload(e.target.checked)}
                className="mt-0.5"
              />
              <span>
                <span className="font-medium">
                  Reload services after install
                </span>
                <span className="block text-xs text-muted-foreground">
                  Sets <code className="font-mono">EXTENSION_RELOAD_MANAGER=1</code>.
                  Only needed when the extension ships systemd units.
                </span>
              </span>
            </label>
          </CardContent>
        </Card>
      )}

      <div className="flex justify-between">
        <Button variant="outline" onClick={onBack}>
          ← Back
        </Button>
        <Button disabled={!required} onClick={onNext}>
          Next →
        </Button>
      </div>
    </div>
  );
}

function TypeCard({
  label,
  desc,
  active,
  onClick,
}: {
  label: string;
  desc: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`text-left rounded-md border p-3 transition-colors ${
        active
          ? "border-[#EE5007] bg-[#EE5007]/5 ring-1 ring-[#EE5007]"
          : "hover:bg-muted/30"
      }`}
    >
      <div className="font-medium text-sm">{label}</div>
      <div className="text-xs text-muted-foreground mt-0.5">{desc}</div>
    </button>
  );
}

function ReviewStep({
  name,
  type,
  arch,
  version,
  sourceMode,
  baseImage,
  selectedArtifactId,
  dockerfile,
  extraSteps,
  hierarchies,
  serviceReload,
  submitting,
  submitErr,
  onBack,
  onSubmit,
}: {
  name: string;
  type: ExtensionType;
  arch: Arch;
  version: string;
  sourceMode: SourceMode;
  baseImage: string;
  selectedArtifactId: string;
  dockerfile: string;
  extraSteps: string;
  hierarchies: string[];
  serviceReload: boolean;
  submitting: boolean;
  submitErr: string | null;
  onBack: () => void;
  onSubmit: () => void;
}) {
  return (
    <div className="grid gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Review</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 text-sm">
          <KV k="Name" v={name} />
          <KV k="Type" v={type} />
          <KV k="Arch" v={arch} />
          <KV k="Version" v={version} />
          <KV k="Source" v={sourceMode} />
          {sourceMode === "artifact" && (
            <KV k="Artifact" v={selectedArtifactId} />
          )}
          {sourceMode === "image" && <KV k="Base image" v={baseImage} />}
          {sourceMode === "dockerfile" && (
            <KV k="Dockerfile" v={`${dockerfile.length} bytes`} />
          )}
          {sourceMode === "artifact" && extraSteps && (
            <KV k="Extra steps" v={`${extraSteps.length} bytes`} />
          )}
          {type === "sysext" && hierarchies.length > 0 && (
            <KV k="Hierarchies" v={hierarchies.join(", ")} />
          )}
          {type === "sysext" && serviceReload && (
            <KV k="Service reload" v="yes" />
          )}
        </CardContent>
      </Card>

      {submitErr && (
        <p role="alert" className="text-sm text-red-600">
          {submitErr}
        </p>
      )}

      <div className="flex justify-between">
        <Button variant="outline" onClick={onBack} disabled={submitting}>
          ← Back
        </Button>
        <Button onClick={onSubmit} disabled={submitting}>
          {submitting ? "Building…" : "Build"}
        </Button>
      </div>
    </div>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <div className="grid grid-cols-[160px_1fr] gap-2">
      <span className="text-muted-foreground">{k}</span>
      <span className="font-mono">{v}</span>
    </div>
  );
}

function StepIndicator({ current }: { current: number }) {
  const steps = ["Source", "Configure", "Review"];
  return (
    <div className="flex gap-3 items-center text-sm mb-6">
      {steps.map((label, i) => (
        <span
          key={label}
          className={`inline-flex items-center gap-1.5 ${
            i === current
              ? "text-[#EE5007] font-semibold"
              : "text-muted-foreground"
          }`}
        >
          <span
            className={`h-6 w-6 rounded-full border inline-flex items-center justify-center text-xs ${
              i === current ? "bg-[#EE5007] text-white border-[#EE5007]" : ""
            }`}
          >
            {i + 1}
          </span>
          {label}
        </span>
      ))}
    </div>
  );
}
