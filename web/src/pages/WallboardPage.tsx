import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type AttentionItem } from "../api/client";
import { Brand } from "../components/Brand";
import { relativeAge } from "../lib/relativeAge";

const TOKEN_KEY = "gitseer-wallboard-token";

/**
 * Public read-only wallboard (token in query or localStorage).
 * localStorage is deliberate for shared-display kiosks (no session cookie); the
 * token is read-only scoped and CSP script-src 'self' keeps XSS risk low.
 */
export function WallboardPage() {
  const [params, setParams] = useSearchParams();
  const queryToken = params.get("token") || "";
  const [stored, setStored] = useState(() => {
    try {
      return localStorage.getItem(TOKEN_KEY) || "";
    } catch {
      return "";
    }
  });
  const [draft, setDraft] = useState("");
  const token = queryToken || stored;

  const snap = useQuery({
    queryKey: ["wallboard-snapshot", token],
    queryFn: () => api.wallboardSnapshot(token),
    enabled: Boolean(token),
    refetchInterval: 30_000,
  });

  const attention = useMemo(() => snap.data?.attention ?? [], [snap.data?.attention]);

  function saveToken() {
    const t = draft.trim();
    if (!t) return;
    try {
      localStorage.setItem(TOKEN_KEY, t);
    } catch {
      /* ignore */
    }
    setStored(t);
    setDraft("");
    const next = new URLSearchParams(params);
    next.delete("token");
    setParams(next, { replace: true });
  }

  if (!token) {
    return (
      <div className="wallboard wallboard--gate">
        <Brand variant="logo" />
        <h1>Wallboard</h1>
        <p className="muted">
          Enter a read-only wallboard token. Tokens are created by a bootstrap admin under Settings → Status.
        </p>
        <div className="wallboard__gate-form">
          <input
            type="password"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Wallboard token"
            aria-label="Wallboard token"
          />
          <button type="button" className="btn primary" onClick={saveToken} disabled={!draft.trim()}>
            Open Wallboard
          </button>
        </div>
        <p className="settings-form__hint">
          Threat model: anyone with the token can read summary and open attention. Tokens do not allow writes,
          sync, settings, or forge credentials. The token is stored in this browser&apos;s localStorage so a
          shared wallboard display stays signed in across reloads — revoke it if the display is compromised.
          Prefer HTTPS and Bearer over query-string tokens when possible.
        </p>
      </div>
    );
  }

  if (snap.isLoading) {
    return (
      <div className="wallboard">
        <div className="loading">Loading wallboard…</div>
      </div>
    );
  }
  if (snap.isError) {
    return (
      <div className="wallboard wallboard--gate">
        <p className="error" role="alert">
          {(snap.error as Error).message || "Could not load wallboard"}
        </p>
        <button
          type="button"
          className="btn"
          onClick={() => {
            try {
              localStorage.removeItem(TOKEN_KEY);
            } catch {
              /* ignore */
            }
            setStored("");
          }}
        >
          Clear Token
        </button>
      </div>
    );
  }

  const s = snap.data!.summary;

  return (
    <div className="wallboard">
      <header className="wallboard__header">
        <Brand variant="logo" />
        <div>
          <h1>Wallboard</h1>
          <p className="muted">Updated {relativeAge(snap.data!.generated_at)} · read-only</p>
        </div>
      </header>

      <div className="metrics">
        <div className="metric">
          <div className="metric__label">Repositories</div>
          <div className="metric__value">{s.repositories}</div>
        </div>
        <div className="metric">
          <div className="metric__label">Open PRs</div>
          <div className="metric__value">{s.open_pull_requests}</div>
        </div>
        <div className="metric">
          <div className="metric__label">Attention</div>
          <div className="metric__value">{s.attention_open}</div>
        </div>
        <div className="metric">
          <div className="metric__label">Failed Runs</div>
          <div className="metric__value">{s.failed_runs}</div>
        </div>
        <div className="metric">
          <div className="metric__label">Running</div>
          <div className="metric__value">{s.running_runs}</div>
        </div>
      </div>

      <section className="report-section">
        <h2>Open Attention</h2>
        {attention.length === 0 ? (
          <div className="empty">No open attention.</div>
        ) : (
          <ul className="wallboard__attention">
            {attention.map((item: AttentionItem) => (
              <li key={item.id} className={`wallboard__attention-item wallboard__attention-item--${item.severity}`}>
                <strong>{item.severity}</strong> {item.repo_full || "—"} — {item.title}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
