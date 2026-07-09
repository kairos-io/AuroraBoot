import { useState } from "react";
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
import { Loader2 } from "lucide-react";
import { createHadronArtifact, type CreateHadronArtifactInput } from "@/api/hadron";

interface HadronPushDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Original hadron spec — will be replayed with push=true. */
  sourceSpec: CreateHadronArtifactInput;
  /** Name of the source artifact so the derived push artifact has a hint. */
  sourceName: string;
  /** Called with the new artifact id after a successful submit. */
  onPushed: (newArtifactId: string) => void;
}

// HadronPushDialog kicks off a new hadron build that clones the source spec
// but flips push=true (and disables the tarball to avoid producing another
// disk artifact). Since the Dockerfile is deterministic, buildkit reuses the
// existing layer cache and the "push" mostly copies bytes rather than
// rebuilding — but it does re-invoke buildx, so the operator still sees a
// full build entry for the pushed artifact.
export function HadronPushDialog({
  open,
  onOpenChange,
  sourceSpec,
  sourceName,
  onPushed,
}: HadronPushDialogProps) {
  const [ref, setRef] = useState(sourceSpec.outputRef || "");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit() {
    const trimmed = ref.trim();
    if (!trimmed) return;
    setSubmitting(true);
    setError(null);
    try {
      const artifact = await createHadronArtifact(
        sourceName ? `push: ${sourceName}` : "",
        {
          ...sourceSpec,
          outputRef: trimmed,
          push: true,
          produceTarball: false,
        }
      );
      onPushed(artifact.id);
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Push Hadron image to registry</DialogTitle>
          <DialogDescription>
            Re-runs the same hadron build with buildkit's cache and pushes
            the result to the ref you provide. Credentials come from Settings →
            Hadron Registry Credentials (add one for the target registry
            first if you haven't).
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2 py-2">
          <Label className="text-xs">Target image ref</Label>
          <Input
            className="font-mono text-xs"
            placeholder="registry.example.com/team/hadron:v1"
            value={ref}
            onChange={(e) => setRef(e.target.value)}
            autoFocus
          />
          <p className="text-[11px] text-muted-foreground">
            Multi-arch: buildx re-uses the cached layers per platform and
            pushes a fat manifest.
          </p>
          {error && (
            <p className="text-xs text-red-600 break-all">{error}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            Cancel
          </Button>
          <Button
            className="bg-[#EE5007] hover:bg-[#FF7442] text-white"
            onClick={handleSubmit}
            disabled={submitting || !ref.trim()}
          >
            {submitting && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Push
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
