import { useEffect, useId, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Organization } from "../api/client";
import type { ForgeChipOption } from "../hooks/useShowForgeUI";
import { Glyph } from "./Glyph";

type Props = {
  orgId: number | null;
  onOrgId: (next: number | null, label?: string) => void;
  forgeFilter: string;
  onForgeFilter: (next: string) => void;
  showForge: boolean;
  forgeOptions: ForgeChipOption[];
};

function orgLabel(org: Organization): string {
  return (org.full_name || org.name || "").trim() || `Org #${org.id}`;
}

/** Filter-icon flyout for dashboard organization and forge scope. */
export function DashboardFilter({
  orgId,
  onOrgId,
  forgeFilter,
  onForgeFilter,
  showForge,
  forgeOptions,
}: Props) {
  const menuId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const active = orgId != null || (showForge && forgeFilter !== "all");

  const orgs = useQuery({
    queryKey: ["organizations"],
    queryFn: () => api.organizations(),
    staleTime: 60_000,
    enabled: open || orgId != null,
  });

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const items = orgs.data?.items ?? [];

  return (
    <div
      className={`dashboard-filter${open ? " is-open" : ""}${active ? " is-active" : ""}`}
      ref={rootRef}
    >
      <button
        type="button"
        className="dashboard-filter__trigger"
        aria-label="Filter Dashboard"
        title="Filter Dashboard"
        aria-expanded={open}
        aria-haspopup="dialog"
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((v) => !v)}
      >
        <Glyph name="filter" />
      </button>
      {open && (
        <div id={menuId} className="dashboard-filter__menu" role="dialog" aria-label="Dashboard filters">
          <div className="dashboard-filter__section">
            <div className="dashboard-filter__heading">Organizations</div>
            {orgs.isLoading ? (
              <div className="dashboard-filter__hint muted">Loading…</div>
            ) : orgs.isError ? (
              <div className="dashboard-filter__hint error">Could not load organizations.</div>
            ) : (
              <>
                <button
                  type="button"
                  className={`dashboard-filter__option${orgId == null ? " is-active" : ""}`}
                  aria-pressed={orgId == null}
                  onClick={() => onOrgId(null)}
                >
                  All Organizations
                </button>
                {items.length === 0 ? (
                  <div className="dashboard-filter__hint muted">No organizations yet.</div>
                ) : (
                  items.map((org) => {
                    const selected = orgId === org.id;
                    const label = orgLabel(org);
                    return (
                      <button
                        key={org.id}
                        type="button"
                        className={`dashboard-filter__option${selected ? " is-active" : ""}`}
                        aria-pressed={selected}
                        onClick={() => onOrgId(org.id, label)}
                      >
                        {label}
                      </button>
                    );
                  })
                )}
              </>
            )}
          </div>
          {showForge ? (
            <div className="dashboard-filter__section">
              <div className="dashboard-filter__heading">Forge</div>
              {forgeOptions.map((opt) => {
                const selected = forgeFilter === opt.id;
                return (
                  <button
                    key={opt.id}
                    type="button"
                    className={`dashboard-filter__option${selected ? " is-active" : ""}`}
                    aria-pressed={selected}
                    onClick={() => onForgeFilter(opt.id)}
                  >
                    {opt.label}
                  </button>
                );
              })}
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
