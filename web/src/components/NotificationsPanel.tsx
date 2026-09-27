import { FormEvent, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type NotificationSettings } from "../api/client";

type Draft = NotificationSettings & {
  smtp_password: string;
  slack_webhook_url: string;
  discord_webhook_url: string;
  webhook_url: string;
  clear_smtp_password: boolean;
  clear_slack_webhook: boolean;
  clear_discord_webhook: boolean;
  clear_webhook_url: boolean;
};

function emptyDraft(): Draft {
  return {
    enabled: false,
    min_severity: "critical",
    immediate_enabled: true,
    digest_enabled: false,
    digest_hour_utc: 14,
    smtp_enabled: false,
    smtp_host: "",
    smtp_port: 587,
    smtp_tls_mode: "starttls",
    smtp_from: "",
    smtp_to: "",
    smtp_username: "",
    smtp_password_configured: false,
    slack_enabled: false,
    slack_webhook_configured: false,
    discord_enabled: false,
    discord_webhook_configured: false,
    webhook_enabled: false,
    webhook_url_configured: false,
    smtp_password: "",
    slack_webhook_url: "",
    discord_webhook_url: "",
    webhook_url: "",
    clear_smtp_password: false,
    clear_slack_webhook: false,
    clear_discord_webhook: false,
    clear_webhook_url: false,
  };
}

function fromSettings(s: NotificationSettings): Draft {
  return {
    ...emptyDraft(),
    ...s,
    smtp_password: "",
    slack_webhook_url: "",
    discord_webhook_url: "",
    webhook_url: "",
  };
}

function dirty(a: Draft, b: Draft): boolean {
  const keys: (keyof NotificationSettings)[] = [
    "enabled",
    "min_severity",
    "immediate_enabled",
    "digest_enabled",
    "digest_hour_utc",
    "smtp_enabled",
    "smtp_host",
    "smtp_port",
    "smtp_tls_mode",
    "smtp_from",
    "smtp_to",
    "smtp_username",
    "slack_enabled",
    "discord_enabled",
    "webhook_enabled",
  ];
  for (const k of keys) {
    if (a[k] !== b[k]) return true;
  }
  return (
    a.smtp_password !== "" ||
    a.slack_webhook_url !== "" ||
    a.discord_webhook_url !== "" ||
    a.webhook_url !== "" ||
    a.clear_smtp_password ||
    a.clear_slack_webhook ||
    a.clear_discord_webhook ||
    a.clear_webhook_url
  );
}

type Props = {
  editable: boolean;
};

/** Bootstrap-admin outbound notification settings (SMTP + webhooks). */
export function NotificationsPanel({ editable }: Props) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["notification-settings"],
    queryFn: () => api.notificationSettings(),
    enabled: editable,
  });
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [baseline, setBaseline] = useState<Draft>(emptyDraft);
  const [error, setError] = useState("");
  const [flash, setFlash] = useState("");

  useEffect(() => {
    if (!q.data?.settings) return;
    const next = fromSettings(q.data.settings);
    setDraft(next);
    setBaseline(next);
  }, [q.data]);

  const isDirty = dirty(draft, baseline);

  const save = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        enabled: draft.enabled,
        min_severity: draft.min_severity,
        immediate_enabled: draft.immediate_enabled,
        digest_enabled: draft.digest_enabled,
        digest_hour_utc: draft.digest_hour_utc,
        smtp_enabled: draft.smtp_enabled,
        smtp_host: draft.smtp_host,
        smtp_port: draft.smtp_port,
        smtp_tls_mode: draft.smtp_tls_mode,
        smtp_from: draft.smtp_from,
        smtp_to: draft.smtp_to,
        smtp_username: draft.smtp_username,
        slack_enabled: draft.slack_enabled,
        discord_enabled: draft.discord_enabled,
        webhook_enabled: draft.webhook_enabled,
      };
      if (draft.clear_smtp_password) body.clear_smtp_password = true;
      else if (draft.smtp_password) body.smtp_password = draft.smtp_password;
      if (draft.clear_slack_webhook) body.clear_slack_webhook = true;
      else if (draft.slack_webhook_url) body.slack_webhook_url = draft.slack_webhook_url;
      if (draft.clear_discord_webhook) body.clear_discord_webhook = true;
      else if (draft.discord_webhook_url) body.discord_webhook_url = draft.discord_webhook_url;
      if (draft.clear_webhook_url) body.clear_webhook_url = true;
      else if (draft.webhook_url) body.webhook_url = draft.webhook_url;
      return api.updateNotificationSettings(body);
    },
    onSuccess: (data) => {
      setError("");
      setFlash("Saved");
      const next = fromSettings(data.settings);
      setDraft(next);
      setBaseline(next);
      void qc.invalidateQueries({ queryKey: ["notification-settings"] });
    },
    onError: (err: Error) => {
      setError(err.message || "Save failed");
      setFlash("");
    },
  });

  const testMut = useMutation({
    mutationFn: () => api.sendTestNotification(),
    onSuccess: (data) => {
      setError("");
      setFlash(`Queued ${data.queued} test notification(s)`);
    },
    onError: (err: Error) => {
      setError(err.message || "Test failed");
      setFlash("");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!editable || !isDirty || save.isPending) return;
    save.mutate();
  }

  function setField<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((d) => ({ ...d, [key]: value }));
    setFlash("");
  }

  if (!editable) {
    return (
      <div className="panel panel--padded">
        <p className="settings-form__hint">Notification settings are editable by the bootstrap admin only.</p>
      </div>
    );
  }

  if (q.isLoading) {
    return (
      <div className="panel panel--padded">
        <p className="settings-form__hint">Loading notification settings…</p>
      </div>
    );
  }

  if (q.isError) {
    return (
      <div className="panel panel--padded">
        <p className="form-error">{(q.error as Error).message || "Failed to load"}</p>
      </div>
    );
  }

  return (
    <form className="panel panel--padded settings-form" onSubmit={onSubmit}>
      <fieldset className="settings-form__section" disabled={save.isPending}>
        <legend>Delivery</legend>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(e) => setField("enabled", e.target.checked)}
            />{" "}
            Enable outbound notifications
          </label>
          <p className="settings-form__hint">
            Self-hosted SMTP and/or webhooks only — no ncdLabs-hosted relay (ADR-015).
          </p>
        </div>
        <div className="settings-form__field">
          <select
            id="min_severity"
            value={draft.min_severity}
            onChange={(e) => setField("min_severity", e.target.value)}
            aria-label="Minimum severity"
          >
            <option value="critical">Critical</option>
            <option value="warning">Warning and above</option>
            <option value="waiting">Waiting and above</option>
          </select>
          <p className="settings-form__hint">Immediate alerts and digests include items at this severity or higher.</p>
        </div>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.immediate_enabled}
              onChange={(e) => setField("immediate_enabled", e.target.checked)}
            />{" "}
            Immediate on new attention
          </label>
        </div>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.digest_enabled}
              onChange={(e) => setField("digest_enabled", e.target.checked)}
            />{" "}
            Daily digest
          </label>
        </div>
        <div className="settings-form__field">
          <input
            id="digest_hour_utc"
            type="number"
            min={0}
            max={23}
            value={draft.digest_hour_utc}
            onChange={(e) => setField("digest_hour_utc", Number(e.target.value))}
            placeholder="Digest hour (UTC)"
            aria-label="Digest hour (UTC)"
            disabled={!draft.digest_enabled}
          />
          <p className="settings-form__hint">UTC hour (0–23) when the daily digest is enqueued.</p>
        </div>
      </fieldset>

      <fieldset className="settings-form__section" disabled={save.isPending}>
        <legend>SMTP</legend>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.smtp_enabled}
              onChange={(e) => setField("smtp_enabled", e.target.checked)}
            />{" "}
            Enable SMTP
          </label>
        </div>
        <div className="settings-form__field">
          <input
            type="text"
            value={draft.smtp_host}
            onChange={(e) => setField("smtp_host", e.target.value)}
            placeholder="SMTP host"
            aria-label="SMTP host"
            autoComplete="off"
          />
        </div>
        <div className="settings-form__grid">
          <div className="settings-form__field">
            <input
              type="number"
              min={1}
              max={65535}
              value={draft.smtp_port}
              onChange={(e) => setField("smtp_port", Number(e.target.value))}
              placeholder="Port"
              aria-label="SMTP port"
            />
          </div>
          <div className="settings-form__field">
            <select
              value={draft.smtp_tls_mode}
              onChange={(e) => setField("smtp_tls_mode", e.target.value)}
              aria-label="SMTP TLS mode"
            >
              <option value="starttls">STARTTLS</option>
              <option value="tls">TLS</option>
              <option value="none">None</option>
            </select>
          </div>
        </div>
        <div className="settings-form__field">
          <input
            type="text"
            value={draft.smtp_from}
            onChange={(e) => setField("smtp_from", e.target.value)}
            placeholder="From address"
            aria-label="SMTP from"
            autoComplete="off"
          />
        </div>
        <div className="settings-form__field">
          <input
            type="text"
            value={draft.smtp_to}
            onChange={(e) => setField("smtp_to", e.target.value)}
            placeholder="To addresses (comma-separated)"
            aria-label="SMTP to"
            autoComplete="off"
          />
        </div>
        <div className="settings-form__field">
          <input
            type="text"
            value={draft.smtp_username}
            onChange={(e) => setField("smtp_username", e.target.value)}
            placeholder="SMTP username"
            aria-label="SMTP username"
            autoComplete="off"
          />
        </div>
        <div className="settings-form__field">
          <input
            type="password"
            value={draft.smtp_password}
            onChange={(e) => {
              setField("smtp_password", e.target.value);
              setField("clear_smtp_password", false);
            }}
            placeholder={
              draft.smtp_password_configured && !draft.clear_smtp_password
                ? "Password configured (leave blank to keep)"
                : "SMTP password"
            }
            aria-label="SMTP password"
            autoComplete="new-password"
          />
          {draft.smtp_password_configured && (
            <label>
              <input
                type="checkbox"
                checked={draft.clear_smtp_password}
                onChange={(e) => {
                  setField("clear_smtp_password", e.target.checked);
                  if (e.target.checked) setField("smtp_password", "");
                }}
              />{" "}
              Clear stored password
            </label>
          )}
        </div>
      </fieldset>

      <fieldset className="settings-form__section" disabled={save.isPending}>
        <legend>Webhooks</legend>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.slack_enabled}
              onChange={(e) => setField("slack_enabled", e.target.checked)}
            />{" "}
            Slack incoming webhook
          </label>
          <input
            type="password"
            value={draft.slack_webhook_url}
            onChange={(e) => {
              setField("slack_webhook_url", e.target.value);
              setField("clear_slack_webhook", false);
            }}
            placeholder={
              draft.slack_webhook_configured && !draft.clear_slack_webhook
                ? "Webhook URL configured (leave blank to keep)"
                : "Slack webhook URL"
            }
            aria-label="Slack webhook URL"
            autoComplete="off"
          />
          {draft.slack_webhook_configured && (
            <label>
              <input
                type="checkbox"
                checked={draft.clear_slack_webhook}
                onChange={(e) => {
                  setField("clear_slack_webhook", e.target.checked);
                  if (e.target.checked) setField("slack_webhook_url", "");
                }}
              />{" "}
              Clear Slack webhook
            </label>
          )}
        </div>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.discord_enabled}
              onChange={(e) => setField("discord_enabled", e.target.checked)}
            />{" "}
            Discord webhook
          </label>
          <input
            type="password"
            value={draft.discord_webhook_url}
            onChange={(e) => {
              setField("discord_webhook_url", e.target.value);
              setField("clear_discord_webhook", false);
            }}
            placeholder={
              draft.discord_webhook_configured && !draft.clear_discord_webhook
                ? "Webhook URL configured (leave blank to keep)"
                : "Discord webhook URL"
            }
            aria-label="Discord webhook URL"
            autoComplete="off"
          />
          {draft.discord_webhook_configured && (
            <label>
              <input
                type="checkbox"
                checked={draft.clear_discord_webhook}
                onChange={(e) => {
                  setField("clear_discord_webhook", e.target.checked);
                  if (e.target.checked) setField("discord_webhook_url", "");
                }}
              />{" "}
              Clear Discord webhook
            </label>
          )}
        </div>
        <div className="settings-form__field">
          <label>
            <input
              type="checkbox"
              checked={draft.webhook_enabled}
              onChange={(e) => setField("webhook_enabled", e.target.checked)}
            />{" "}
            Generic HTTPS webhook
          </label>
          <input
            type="password"
            value={draft.webhook_url}
            onChange={(e) => {
              setField("webhook_url", e.target.value);
              setField("clear_webhook_url", false);
            }}
            placeholder={
              draft.webhook_url_configured && !draft.clear_webhook_url
                ? "Webhook URL configured (leave blank to keep)"
                : "HTTPS webhook URL"
            }
            aria-label="Generic webhook URL"
            autoComplete="off"
          />
          {draft.webhook_url_configured && (
            <label>
              <input
                type="checkbox"
                checked={draft.clear_webhook_url}
                onChange={(e) => {
                  setField("clear_webhook_url", e.target.checked);
                  if (e.target.checked) setField("webhook_url", "");
                }}
              />{" "}
              Clear generic webhook
            </label>
          )}
          <p className="settings-form__hint">
            POSTs JSON with title, severity, repo, and deep_link. Secrets are write-only and sealed with the encryption key.
          </p>
        </div>
      </fieldset>

      {error ? <p className="form-error">{error}</p> : null}
      {flash ? <p className="settings-form__hint">{flash}</p> : null}

      <div className="settings-form__actions">
        <button
          type="button"
          className="btn btn--ghost"
          disabled={save.isPending || testMut.isPending}
          onClick={() => {
            const next = fromSettings(baseline);
            setDraft(next);
            setError("");
            setFlash("");
          }}
        >
          Cancel
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={save.isPending || testMut.isPending || isDirty}
          onClick={() => testMut.mutate()}
        >
          {testMut.isPending ? "Sending…" : "Send Test Notification"}
        </button>
        <button type="submit" className="btn" disabled={!isDirty || save.isPending}>
          {save.isPending ? "Saving…" : "Save"}
        </button>
      </div>
    </form>
  );
}
