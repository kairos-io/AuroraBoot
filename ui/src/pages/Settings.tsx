import { useEffect, useState } from "react";
import { getRegistrationToken, rotateRegistrationToken } from "@/api/settings";
import {
  listRegistryCredentials,
  putRegistryCredentials,
  type RegistryCredential,
} from "@/api/hadron";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/PageHeader";
import { Eye, EyeOff, RefreshCw, Plus, Trash2, Save, Loader2 } from "lucide-react";
import { toast } from "@/hooks/useToast";

// Local row shape mirrors RegistryCredential but adds a `dirty` marker so we
// know when to send a plaintext password vs preserving the encrypted value
// server-side (keepPassword). Existing rows load with dirty=false; freshly
// added rows start dirty.
type CredentialRow = {
  registry: string;
  username: string;
  password: string;
  hasPassword: boolean;
  dirty: boolean;
};

export function Settings() {
  const [token, setToken] = useState("");
  const [revealed, setRevealed] = useState(false);
  const [rotating, setRotating] = useState(false);

  const [creds, setCreds] = useState<CredentialRow[]>([]);
  const [credsLoading, setCredsLoading] = useState(true);
  const [credsSaving, setCredsSaving] = useState(false);

  useEffect(() => {
    getRegistrationToken()
      .then((t) => setToken(t.token))
      .catch(() => {});
    reloadCreds();
  }, []);

  function reloadCreds() {
    setCredsLoading(true);
    listRegistryCredentials()
      .then((list) => {
        setCreds(
          list.map((c) => ({
            registry: c.registry,
            username: c.username,
            password: "",
            hasPassword: !!c.hasPassword,
            dirty: false,
          }))
        );
      })
      .catch(() => setCreds([]))
      .finally(() => setCredsLoading(false));
  }

  async function handleRotate() {
    if (!confirm("Are you sure? This will invalidate the current token.")) return;
    setRotating(true);
    try {
      const result = await rotateRegistrationToken();
      setToken(result.token);
    } finally {
      setRotating(false);
    }
  }

  function updateRow(idx: number, patch: Partial<CredentialRow>) {
    setCreds((cur) => cur.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  }

  function addRow() {
    setCreds((cur) => [
      ...cur,
      { registry: "", username: "", password: "", hasPassword: false, dirty: true },
    ]);
  }

  function removeRow(idx: number) {
    setCreds((cur) => cur.filter((_, i) => i !== idx));
  }

  async function saveCreds() {
    // Payload rules:
    //  - registry + username always sent verbatim.
    //  - When the user typed a new password, send it (plaintext → backend encrypts).
    //  - When the row already has a stored password and the user didn't touch it,
    //    send keepPassword=true so the backend preserves the ciphertext byte-for-byte.
    //  - Otherwise omit password entirely (clears the stored value).
    const payload: RegistryCredential[] = creds.map((r) => {
      const base: RegistryCredential = { registry: r.registry.trim(), username: r.username.trim() };
      if (r.password.trim()) {
        base.password = r.password;
      } else if (r.hasPassword && !r.dirty) {
        base.keepPassword = true;
      }
      return base;
    });
    setCredsSaving(true);
    try {
      await putRegistryCredentials(payload);
      toast("Registry credentials saved", "success");
      reloadCreds();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast(`Failed to save credentials: ${msg}`, "error");
    } finally {
      setCredsSaving(false);
    }
  }

  const maskedToken = token ? token.slice(0, 8) + "..." + token.slice(-4) : "";
  const canSaveCreds =
    !credsSaving &&
    creds.every((r) => r.registry.trim() !== "" && r.username.trim() !== "");

  return (
    <div>
      <PageHeader title="Settings" description="Server configuration and tokens" />

      <div className="grid gap-6 max-w-2xl">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Registration Token</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4">
            <p className="text-sm text-muted-foreground">
              This token is used by nodes to register with the server. Keep it secret.
            </p>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <Input
                  readOnly
                  value={revealed ? token : maskedToken}
                  className="font-mono pr-10"
                />
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full"
                  onClick={() => setRevealed(!revealed)}
                >
                  {revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              <Button variant="outline" onClick={handleRotate} disabled={rotating}>
                <RefreshCw className={`h-4 w-4 mr-2 ${rotating ? "animate-spin" : ""}`} />
                Rotate
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Hadron Registry Credentials</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Credentials used by hadron builds when pushing images. Passwords are stored encrypted; leaving the password field empty on save preserves the previously stored value.
            </p>
            {credsLoading ? (
              <p className="text-xs italic text-muted-foreground">Loading…</p>
            ) : (
              <div className="space-y-2">
                {creds.map((row, idx) => (
                  <div key={idx} className="grid grid-cols-[1fr_1fr_1fr_auto] gap-2 items-center">
                    <Input
                      placeholder="registry.example.com"
                      value={row.registry}
                      onChange={(e) => updateRow(idx, { registry: e.target.value })}
                      className="font-mono text-xs"
                    />
                    <Input
                      placeholder="username"
                      value={row.username}
                      onChange={(e) => updateRow(idx, { username: e.target.value })}
                      className="font-mono text-xs"
                    />
                    <Input
                      type="password"
                      placeholder={row.hasPassword ? "•••••••• (unchanged)" : "password"}
                      value={row.password}
                      onChange={(e) => updateRow(idx, { password: e.target.value, dirty: true })}
                      className="font-mono text-xs"
                    />
                    <Button variant="ghost" size="icon" onClick={() => removeRow(idx)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
                <div className="flex gap-2 pt-1">
                  <Button variant="outline" size="sm" onClick={addRow}>
                    <Plus className="h-4 w-4 mr-1" /> Add credential
                  </Button>
                  <Button size="sm" onClick={saveCreds} disabled={!canSaveCreds}>
                    {credsSaving ? <Loader2 className="h-4 w-4 mr-1 animate-spin" /> : <Save className="h-4 w-4 mr-1" />}
                    Save
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
