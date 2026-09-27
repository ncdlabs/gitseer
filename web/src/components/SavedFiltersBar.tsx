import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, type SavedFilter, type SavedFilterQuery } from "../api/client";

type Props = {
  /** Restrict presets to this page value (or missing page). */
  page: "attention" | "inbox" | "pull-requests";
  /** Current list query string to save. */
  currentQuery: SavedFilterQuery;
  /** Called when a preset is applied (parent may sync local filter state). */
  onApply?: (query: SavedFilterQuery) => void;
};

function pathForFilter(query: SavedFilterQuery, fallback: Props["page"]): string {
  const page = (query.page || fallback) as Props["page"];
  const params = new URLSearchParams();
  if (query.q) params.set("q", query.q);
  if (query.severity) params.set("severity", query.severity);
  if (query.type) params.set("type", query.type);
  if (query.forge_type) params.set("forge_type", query.forge_type);
  if (query.instance_id && query.instance_id > 0) params.set("instance_id", String(query.instance_id));
  if (query.reason) params.set("reason", query.reason);
  const qs = params.toString();
  const base =
    page === "inbox" ? "/inbox" : page === "pull-requests" ? "/pull-requests" : "/attention";
  return qs ? `${base}?${qs}` : base;
}

function matchesPage(f: SavedFilter, page: Props["page"]): boolean {
  const p = f.query?.page;
  if (!p) return true;
  return p === page;
}

/** Preset chips for Attention / Inbox / header-adjacent filter saves. */
export function SavedFiltersBar({ page, currentQuery, onApply }: Props) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const list = useQuery({
    queryKey: ["saved-filters"],
    queryFn: api.savedFilters,
  });

  const filters = useMemo(
    () => (list.data?.items ?? []).filter((f) => matchesPage(f, page)),
    [list.data?.items, page],
  );

  const create = useMutation({
    mutationFn: () => api.createSavedFilter(name.trim(), { ...currentQuery, page }),
    onSuccess: async () => {
      setName("");
      setError("");
      setSaving(false);
      await queryClient.invalidateQueries({ queryKey: ["saved-filters"] });
    },
    onError: (err: Error) => {
      setSaving(false);
      setError(err.message || "Could not save filter");
    },
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteSavedFilter(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["saved-filters"] });
    },
  });

  function apply(f: SavedFilter) {
    const q = f.query || {};
    onApply?.(q);
    navigate(pathForFilter(q, page));
  }

  function onSave() {
    if (!name.trim() || create.isPending) return;
    setSaving(true);
    setError("");
    create.mutate();
  }

  return (
    <div className="saved-filters" aria-label="Saved filters">
      <div className="saved-filters__presets forge-filter" role="group" aria-label="Filter presets">
        {filters.length === 0 ? (
          <span className="saved-filters__empty muted">No saved filters</span>
        ) : (
          filters.map((f) => (
            <span key={f.id} className="saved-filters__preset">
              <button
                type="button"
                className="forge-filter__chip"
                onClick={() => apply(f)}
                title={`Apply ${f.name}`}
              >
                {f.name}
              </button>
              <button
                type="button"
                className="saved-filters__remove"
                aria-label={`Delete ${f.name}`}
                title={`Delete ${f.name}`}
                disabled={remove.isPending}
                onClick={() => remove.mutate(f.id)}
              >
                ×
              </button>
            </span>
          ))
        )}
      </div>
      <div className="saved-filters__save">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Filter name"
          aria-label="Saved filter name"
          maxLength={80}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              onSave();
            }
          }}
        />
        <button type="button" disabled={!name.trim() || saving || create.isPending} onClick={onSave}>
          Save Filter
        </button>
      </div>
      {error && <div className="error saved-filters__error">{error}</div>}
    </div>
  );
}
