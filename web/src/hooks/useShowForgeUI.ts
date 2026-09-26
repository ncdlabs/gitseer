import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  api,
  forgeLabel,
  type ForgeStatusRow,
  type SystemStatus,
} from "../api/client";

/** Count forges marked configured in system status (forges[] or legacy flags). */
export function configuredForgeCount(status?: SystemStatus | null): number {
  if (!status) return 0;
  const forges = Array.isArray(status.forges) ? status.forges : [];
  if (forges.length > 0) {
    return forges.filter((f) => f.configured).length;
  }
  return (status.gitea_configured ? 1 : 0) + (status.github_configured ? 1 : 0);
}

export function configuredForges(status?: SystemStatus | null): ForgeStatusRow[] {
  if (!status) return [];
  const forges = Array.isArray(status.forges) ? status.forges : [];
  if (forges.length > 0) {
    return forges.filter((f) => f.configured);
  }
  const legacy: ForgeStatusRow[] = [];
  if (status.gitea_configured) {
    legacy.push({ forge_type: "gitea", configured: true, name: "Gitea" });
  }
  if (status.github_configured) {
    legacy.push({ forge_type: "github", configured: true, name: "GitHub" });
  }
  return legacy;
}

export type ForgeChipOption = {
  /** "all" | forge type | "instance:{id}" */
  id: string;
  label: string;
  forgeType?: string;
  instanceId?: number;
};

/** Build All + type chips; when a type has multiple instances, add per-instance chips. */
export function buildForgeChipOptions(forges: ForgeStatusRow[]): ForgeChipOption[] {
  const opts: ForgeChipOption[] = [{ id: "all", label: "All" }];
  if (forges.length === 0) return opts;

  const byType = new Map<string, ForgeStatusRow[]>();
  for (const f of forges) {
    const ft = (f.forge_type || "gitea").toLowerCase();
    const list = byType.get(ft) ?? [];
    list.push(f);
    byType.set(ft, list);
  }

  const typeOrder = ["gitea", "github", ...[...byType.keys()].filter((k) => k !== "gitea" && k !== "github")];
  for (const ft of typeOrder) {
    const list = byType.get(ft);
    if (!list || list.length === 0) continue;
    opts.push({ id: ft, label: forgeLabel(ft), forgeType: ft });
    if (list.length > 1) {
      for (const inst of list) {
        const id = inst.instance_id;
        if (id == null || id <= 0) continue;
        const name = (inst.name || "").trim() || `#${id}`;
        opts.push({
          id: `instance:${id}`,
          label: name,
          forgeType: ft,
          instanceId: id,
        });
      }
    }
  }
  return opts;
}

export function typeHasMultipleInstances(forges: ForgeStatusRow[], forgeType?: string | null): boolean {
  const ft = (forgeType || "").toLowerCase();
  if (!ft) return false;
  return forges.filter((f) => (f.forge_type || "").toLowerCase() === ft).length > 1;
}

export function resolveInstanceName(
  forges: ForgeStatusRow[],
  opts: { forgeType?: string | null; instanceId?: number | null; instanceName?: string | null },
): string | undefined {
  if (!typeHasMultipleInstances(forges, opts.forgeType)) return undefined;
  const direct = (opts.instanceName || "").trim();
  if (direct) return direct;
  const id = opts.instanceId;
  if (id == null || id <= 0) return undefined;
  const row = forges.find((f) => f.instance_id === id);
  return (row?.name || "").trim() || undefined;
}

/**
 * True when 2+ forges are configured — show forge badges and filter chips.
 * Hidden for 0–1 configured forges (noise).
 */
export function useShowForgeUI(): boolean {
  const q = useQuery({
    queryKey: ["system-status"],
    queryFn: api.systemStatus,
    staleTime: 60_000,
  });
  return configuredForgeCount(q.data) >= 2;
}

/** Configured forges + chip options for inventory filters. */
export function useForgeInventory() {
  const q = useQuery({
    queryKey: ["system-status"],
    queryFn: api.systemStatus,
    staleTime: 60_000,
  });
  const forges = useMemo(() => configuredForges(q.data), [q.data]);
  const showForge = forges.length >= 2;
  const chipOptions = useMemo(() => buildForgeChipOptions(forges), [forges]);
  return {
    forges,
    showForge,
    chipOptions,
    status: q.data,
    isLoading: q.isLoading,
  };
}
