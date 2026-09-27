import { FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";

/** Bootstrap-admin wallboard token management (Settings → Status). */
export function WallboardTokensPanel() {
  const qc = useQueryClient();
  const [name, setName] = useState("Lobby Display");
  const [secretOnce, setSecretOnce] = useState("");
  const [error, setError] = useState("");

  const list = useQuery({
    queryKey: ["wallboard-tokens"],
    queryFn: () => api.wallboardTokens(false),
  });

  const create = useMutation({
    mutationFn: () => api.createWallboardToken(name.trim()),
    onSuccess: async (data) => {
      setSecretOnce(data.secret);
      setError("");
      await qc.invalidateQueries({ queryKey: ["wallboard-tokens"] });
    },
    onError: (err: Error) => setError(err.message || "Create failed"),
  });

  const revoke = useMutation({
    mutationFn: (id: number) => api.revokeWallboardToken(id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["wallboard-tokens"] });
    },
  });

  function onCreate(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    create.mutate();
  }

  const wallboardPath =
    typeof window !== "undefined"
      ? `${window.location.origin}${(window.__GITSEER_BASE__ || "").replace(/\/$/, "")}/wallboard`
      : "/wallboard";

  return (
    <div className="panel panel--padded">
      <h3 className="settings-status__title">Public Wallboard</h3>
      <p className="settings-form__hint">
        Issue a read-only token for{" "}
        <a href={wallboardPath} target="_blank" rel="noreferrer">
          {wallboardPath}
        </a>
        . The bearer can view summary and open attention only — no write ops, settings, or secrets. Revoke if
        a display is compromised. Prefer the Authorization Bearer header over putting the token in the URL
        query string (query tokens may appear in access logs and Referer headers). Prefer HTTPS.
      </p>
      <form className="settings-form__field" onSubmit={onCreate}>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Token name"
          aria-label="Wallboard token name"
        />
        <button type="submit" className="btn primary" disabled={create.isPending || !name.trim()}>
          Create Token
        </button>
      </form>
      {error ? <p className="error">{error}</p> : null}
      {secretOnce ? (
        <p className="settings-form__hint" role="status">
          Copy this secret now (shown once): <code>{secretOnce}</code>
        </p>
      ) : null}
      {list.isLoading ? (
        <p className="muted">Loading tokens…</p>
      ) : (
        <ul className="wallboard-token-list">
          {(list.data?.items ?? []).map((t) => (
            <li key={t.id}>
              <strong>{t.name}</strong> <span className="muted">{t.token_prefix}…</span>{" "}
              <button type="button" className="btn btn--small" onClick={() => revoke.mutate(t.id)}>
                Revoke
              </button>
            </li>
          ))}
          {(list.data?.items ?? []).length === 0 ? <li className="muted">No active tokens.</li> : null}
        </ul>
      )}
    </div>
  );
}
