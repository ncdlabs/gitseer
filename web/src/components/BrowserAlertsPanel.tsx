import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import {
  disableBrowserAlerts,
  enableBrowserAlerts,
  notificationPermission,
  pushSupported,
} from "../lib/browserAlerts";

export function BrowserAlertsPanel() {
  const queryClient = useQueryClient();
  const prefsQuery = useQuery({ queryKey: ["alert-prefs"], queryFn: api.alertPrefs });
  const [perm, setPerm] = useState(notificationPermission());
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setPerm(notificationPermission());
  }, [prefsQuery.dataUpdatedAt]);

  const enableMut = useMutation({
    mutationFn: enableBrowserAlerts,
    onSuccess: () => {
      setError(null);
      setMessage("Browser and OS alerts enabled for this device.");
      setPerm(notificationPermission());
      void queryClient.invalidateQueries({ queryKey: ["alert-prefs"] });
    },
    onError: (err: Error) => {
      setMessage(null);
      setError(err.message || "Could not enable alerts");
      setPerm(notificationPermission());
    },
  });

  const disableMut = useMutation({
    mutationFn: disableBrowserAlerts,
    onSuccess: () => {
      setError(null);
      setMessage("Browser and OS alerts disabled on this device.");
      void queryClient.invalidateQueries({ queryKey: ["alert-prefs"] });
    },
    onError: (err: Error) => {
      setMessage(null);
      setError(err.message || "Could not disable alerts");
    },
  });

  const severityMut = useMutation({
    mutationFn: (min_severity: string) => api.updateAlertPrefs({ min_severity }),
    onSuccess: () => {
      setError(null);
      void queryClient.invalidateQueries({ queryKey: ["alert-prefs"] });
    },
    onError: (err: Error) => setError(err.message || "Could not save severity"),
  });

  const testMut = useMutation({
    mutationFn: api.sendTestBrowserAlert,
    onSuccess: (res) => {
      setError(null);
      setMessage(`Sent test push to ${res.sent} subscription(s).`);
    },
    onError: (err: Error) => {
      setMessage(null);
      setError(err.message || "Test push failed");
    },
  });

  const prefs = prefsQuery.data?.prefs;
  const enabled = Boolean(prefs?.browser_enabled || prefs?.push_enabled);
  const busy = enableMut.isPending || disableMut.isPending || testMut.isPending || severityMut.isPending;

  return (
    <section className="panel panel--padded settings-form" aria-labelledby="browser-alerts-heading">
      <fieldset className="settings-form__section">
        <legend id="browser-alerts-heading">Browser &amp; OS Alerts</legend>
        <p className="settings-form__hint">
          Personal alerts for this account. When GitSeer is open, alerts use the browser Notification
          API (OS banners when the tab is in the background). Web Push covers closed tabs via a
          service worker — self-hosted VAPID keys, no external relay.
        </p>

      {prefsQuery.isLoading ? <div className="loading">Loading alert prefs…</div> : null}
      {prefsQuery.isError ? (
        <div className="error">{(prefsQuery.error as Error).message}</div>
      ) : null}

      {prefs ? (
        <>
          <p className="muted">
            Status: {enabled ? "enabled" : "disabled"}
            {perm !== "unsupported" ? ` · permission ${perm}` : " · notifications unsupported"}
            {prefs.push_configured
              ? ` · push ready (${prefs.subscription_count} device${prefs.subscription_count === 1 ? "" : "s"})`
              : " · push keys unavailable"}
            {!pushSupported() ? " · this browser does not support Web Push" : ""}
          </p>

          <div className="settings-form__field">
            <select
              aria-label="Minimum severity for browser alerts"
              value={prefs.min_severity || "critical"}
              disabled={busy}
              onChange={(e) => severityMut.mutate(e.target.value)}
            >
              <option value="critical">Critical only</option>
              <option value="warning">Warning and above</option>
              <option value="waiting">All severities</option>
            </select>
            <p className="settings-form__hint">Minimum attention severity that triggers an alert.</p>
          </div>

          <div className="settings-form__actions">
            {!enabled ? (
              <button
                type="button"
                className="btn primary"
                disabled={busy || perm === "unsupported"}
                onClick={() => enableMut.mutate()}
              >
                Enable Alerts
              </button>
            ) : (
              <button
                type="button"
                className="btn"
                disabled={busy}
                onClick={() => disableMut.mutate()}
              >
                Disable Alerts
              </button>
            )}
            <button
              type="button"
              className="btn"
              disabled={busy || !prefs.push_configured || prefs.subscription_count < 1}
              onClick={() => testMut.mutate()}
            >
              Send Test Push
            </button>
          </div>
        </>
      ) : null}

      {message ? <p className="muted">{message}</p> : null}
      {error ? <div className="error">{error}</div> : null}
      </fieldset>
    </section>
  );
}
