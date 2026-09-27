import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type SavedFilterQuery } from "../api/client";

type Props = {
  owner: string;
  onOwner: (owner: string) => void;
  currentQuery: SavedFilterQuery;
};

/** Org/owner scope control + dashboard saved filter presets. */
export function DashboardScopeBar({ owner, onOwner, currentQuery }: Props) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  const list = useQuery({
    queryKey: ["saved-filters"],
    queryFn: api.savedFilters,
  });

  const filters = useMemo(
    () => (list.data?.items ?? []).filter((f) => !f.query?.page || f.query.page === "dashboard"),
    [list.data?.items],
  );

  const create = useMutation({
    mutationFn: () =>
      api.createSavedFilter(name.trim(), {
        ...currentQuery,
        page: "dashboard",
        owner: owner || undefined,
      }),
    onSuccess: async () => {
      setName("");
      setError("");
      await qc.invalidateQueries({ queryKey: ["saved-filters"] });
    },
    onError: (err: Error) => setError(err.message || "Could not save filter"),
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteSavedFilter(id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["saved-filters"] });
    },
  });

  return (
    <div className="dashboard-scope-bar">
      <label className="dashboard-scope-bar__field">
        <span className="muted">Organization / Owner</span>
        <input
          type="text"
          value={owner}
          placeholder="All organizations"
          aria-label="Organization or owner filter"
          onChange={(e) => onOwner(e.target.value)}
        />
      </label>
      {owner ? (
        <button type="button" className="btn" onClick={() => onOwner("")}>
          Clear Scope
        </button>
      ) : null}
      <div className="saved-filters saved-filters--inline">
        {filters.map((f) => (
          <span key={f.id} className="saved-filters__chip">
            <button
              type="button"
              className="btn btn--small"
              onClick={() => onOwner((f.query?.owner || f.query?.team || "").trim())}
            >
              {f.name}
            </button>
            <button
              type="button"
              className="btn btn--small"
              aria-label={`Delete ${f.name}`}
              onClick={() => remove.mutate(f.id)}
            >
              ×
            </button>
          </span>
        ))}
        <input
          type="text"
          value={name}
          placeholder="Preset name"
          aria-label="Dashboard preset name"
          onChange={(e) => setName(e.target.value)}
        />
        <button
          type="button"
          className="btn btn--small"
          disabled={!name.trim() || create.isPending}
          onClick={() => create.mutate()}
        >
          Save Preset
        </button>
        {error ? <span className="error">{error}</span> : null}
      </div>
      <p className="settings-form__hint">
        Team filter aliases owner login until forges expose team membership in inventory.{" "}
        <Link to="/settings#notifications">Incident webhooks</Link> ·{" "}
        <Link to="/settings#status">Wallboard tokens</Link>
      </p>
    </div>
  );
}
