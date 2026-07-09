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

// Local row shape mirrors RegistryCredential but tracks the initial
// (registry, username) tuple so we can tell when a row was renamed. The
// backend keys keepPassword lookups on the current tuple, so if we sent
// keepPassword=true on a renamed row the old ciphertext would be missed and
// the password would silently drop. Instead the save flow refuses to submit
// a renamed row that still carries a stored password until the user re-types
// it (dirty=true).
type CredentialRow = {
  registry: string;
  username: string;
  password: string;
  hasPassword: boolean;
  dirty: boolean;
  initialRegistry: string;
  initialUsername: string;
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
            initialRegistry: c.registry,
            initialUsername: c.username,
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
      {
        registry: "",
        username: "",
        password: "",
        hasPassword: false,
        dirty: true,
        initialRegistry: "",
        initialUsername: "",
      },
    ]);
  }

  function removeRow(idx: number) {
    setCreds((cur) => cur.filter((_, i) => i !== idx));
  }

  // renamedWithoutPassword flags rows whose registry or username was edited
  // from the loaded value but still rely on a stored password — the backend
  // keys keepPassword on the current (registry, username) tuple, so a rename
  // without a fresh password would silently drop the stored value. The Save
  // button is disabled while any such row exists; the row-level hint tells
  // the user to re-enter the password.
  function isRenamedWithoutPassword(r: CredentialRow) {
    if (!r.hasPassword) return false;
    if (r.dirty && r.password.trim()) return false;
    return r.registry.trim() !== r.initialRegistry || r.username.trim() !== r.initialUsername;
  }

  async function saveCreds() {
    // Payload rules:
    //  - registry + username always sent verbatim.
    //  - When the user typed a new password, send it (plaintext → backend encrypts).
    //  - When the row already has a stored password AND the tuple is
    //    unchanged AND the user didn't touch the password field, send
    //    keepPassword=true so the backend preserves the ciphertext.
    //  - Otherwise omit password entirely (clears the stored value).
    const payload: RegistryCredential[] = creds.map((r) => {
      const base: RegistryCredential = { registry: r.registry.trim(), username: r.username.trim() };
      const tupleUnchanged =
        r.registry.trim() === r.initialRegistry &&
        r.username.trim() === r.initialUsername;
      if (r.password.trim()) {
        base.password = r.password;
      } else if (r.hasPassword && !r.dirty && tupleUnchanged) {
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
    creds.every((r) => r.registry.trim() !== "" && r.username.trim() !== "") &&
    !creds.some(isRenamedWithoutPassword);

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
                {creds.map((row, idx) => {
                  const renameNeedsPassword = isRenamedWithoutPassword(row);
                  return (
                    <div key={idx} className="space-y-1">
                      <div className="grid grid-cols-[1fr_1fr_1fr_auto] gap-2 items-center">
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
                          className={`font-mono text-xs ${renameNeedsPassword ? "border-amber-500 focus-visible:ring-amber-500" : ""}`}
                        />
                        <Button variant="ghost" size="icon" onClick={() => removeRow(idx)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                      {renameNeedsPassword && (
                        <p className="text-[11px] text-amber-600 dark:text-amber-400 pl-1">
                          Registry/username changed on a row with a stored password — re-enter the password to save (the stored ciphertext can't be preserved across a tuple rename).
                        </p>
                      )}
                    </div>
                  );
                })}
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
