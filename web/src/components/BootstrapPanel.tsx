import { FormEvent, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { HiddenUsernameField, PasswordInput } from "./PasswordInput";

type Props = {
  elevatedUntil: string;
};

function remainingLabel(untilISO: string, nowMs: number): string {
  const ms = new Date(untilISO).getTime() - nowMs;
  if (ms <= 0) return "expired";
  const totalSec = Math.ceil(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}m ${String(s).padStart(2, "0")}s`;
}

export function BootstrapPanel({ elevatedUntil }: Props) {
  const qc = useQueryClient();
  const [now, setNow] = useState(() => Date.now());
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [ok, setOk] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setOk(null);
    if (next !== confirm) {
      setError("New passwords do not match");
      return;
    }
    setSaving(true);
    try {
      await api.setBootstrapPassword(current, next);
      setCurrent("");
      setNext("");
      setConfirm("");
      setOk("Bootstrap password updated");
      void qc.invalidateQueries({ queryKey: ["me"] });
    } catch (err) {
      setError(err instanceof Error ? err.message : "update failed");
    } finally {
      setSaving(false);
    }
  }

  const remaining = remainingLabel(elevatedUntil, now);

  return (
    <div
      className="panel panel--padded settings-form"
      id="settings-panel-bootstrap"
      role="tabpanel"
      aria-labelledby="settings-tab-bootstrap"
    >
      <h2 className="settings-status__title">Bootstrap Password</h2>
      <p className="muted">
        Bootstrap access expires in <strong>{remaining}</strong>. Reset the shared bootstrap password
        while this grant is active.
      </p>
      <form className="settings-form__section" onSubmit={onSubmit}>
        <HiddenUsernameField id="bootstrap-username" />
        <div className="settings-form__field">
          <PasswordInput
            id="bootstrap-current"
            autoComplete="current-password"
            placeholder="Current password"
            aria-label="Current bootstrap password"
            value={current}
            onChange={setCurrent}
            required
          />
        </div>
        <div className="settings-form__field">
          <PasswordInput
            id="bootstrap-new"
            autoComplete="new-password"
            placeholder="New password (min 8 characters)"
            aria-label="New bootstrap password"
            value={next}
            onChange={setNext}
            required
          />
        </div>
        <div className="settings-form__field">
          <PasswordInput
            id="bootstrap-confirm"
            autoComplete="new-password"
            placeholder="Confirm new password"
            aria-label="Confirm new bootstrap password"
            value={confirm}
            onChange={setConfirm}
            required
          />
        </div>
        {error && <p className="error">{error}</p>}
        {ok && <p className="muted">{ok}</p>}
        <div className="settings-form__actions">
          <button className="btn primary" type="submit" disabled={saving || !current || !next || !confirm}>
            {saving ? "Saving…" : "Update Password"}
          </button>
        </div>
      </form>
    </div>
  );
}
