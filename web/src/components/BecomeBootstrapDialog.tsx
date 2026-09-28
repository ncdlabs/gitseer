import { FormEvent, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { HiddenUsernameField, PasswordInput } from "./PasswordInput";

type Props = {
  open: boolean;
  onClose: () => void;
  onGranted: (until: string, message: string) => void;
};

export function BecomeBootstrapDialog({ open, onClose, onGranted }: Props) {
  const qc = useQueryClient();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!open) return null;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const out = await api.elevateBootstrap(password);
      setPassword("");
      void qc.invalidateQueries({ queryKey: ["me"] });
      void qc.invalidateQueries({ queryKey: ["settings"] });
      onGranted(out.bootstrap_elevated_until, out.message || "Bootstrap access granted for 5 minutes");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "elevation failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="become-bootstrap-title"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="become-bootstrap-title">Become Bootstrap</h2>
        <p className="muted">
          Enter the bootstrap password to grant admin access for 5 minutes (like sudo).
        </p>
        <form onSubmit={submit}>
          <HiddenUsernameField id="elevate-username" />
          <PasswordInput
            id="elevate-password"
            autoComplete="current-password"
            placeholder="Bootstrap password"
            aria-label="Bootstrap password"
            value={password}
            onChange={setPassword}
            required
          />
          {error && <p className="error">{error}</p>}
          <div className="modal__actions">
            <button type="button" className="btn" onClick={onClose} disabled={busy}>
              Cancel
            </button>
            <button type="submit" className="btn primary" disabled={busy || !password}>
              {busy ? "Granting…" : "Grant Access"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
