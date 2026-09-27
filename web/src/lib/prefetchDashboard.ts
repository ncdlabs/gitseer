import type { QueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import {
  DASHBOARD_RANGE_OPTIONS,
  type DashboardRangeDays,
} from "../hooks/useDashboardRange";

const STALE_TIME = 60_000;

export type PrefetchDashboardOptions = {
  preferDays?: number;
};

function isRangeDays(n: number): n is DashboardRangeDays {
  return (DASHBOARD_RANGE_OPTIONS as readonly number[]).includes(n);
}

/** Prefer current range first, then remaining presets in DASHBOARD_RANGE_OPTIONS order. */
function rangeOrder(preferDays?: number): DashboardRangeDays[] {
  const prefer = preferDays != null && isRangeDays(preferDays) ? preferDays : undefined;
  const rest = DASHBOARD_RANGE_OPTIONS.filter((d) => d !== prefer);
  return prefer !== undefined ? [prefer, ...rest] : [...DASHBOARD_RANGE_OPTIONS];
}

/**
 * Prefetch dashboard layers for every range preset: summary+core first,
 * then trends (non-Now), then duration last. Does not block first paint.
 */
export async function prefetchDashboardRanges(
  queryClient: QueryClient,
  options: PrefetchDashboardOptions = {},
): Promise<void> {
  const ordered = rangeOrder(options.preferDays);

  await Promise.all(
    ordered.flatMap((days) => [
      queryClient.prefetchQuery({
        queryKey: ["summary", days],
        queryFn: () => api.summary(days),
        staleTime: STALE_TIME,
      }),
      queryClient.prefetchQuery({
        queryKey: ["stats", days, "core"],
        queryFn: () => api.stats(days, "core"),
        staleTime: STALE_TIME,
      }),
    ]),
  );

  const trendDays = ordered.filter((d) => d !== 0);
  await Promise.all(
    trendDays.map((days) =>
      queryClient.prefetchQuery({
        queryKey: ["stats", days, "trends"],
        queryFn: () => api.stats(days, "trends"),
        staleTime: STALE_TIME,
      }),
    ),
  );

  // Serial duration so the heavy percentile scan does not stampede the DB.
  for (const days of trendDays) {
    await queryClient.prefetchQuery({
      queryKey: ["stats", days, "duration"],
      queryFn: () => api.stats(days, "duration"),
      staleTime: STALE_TIME,
    });
  }
}
