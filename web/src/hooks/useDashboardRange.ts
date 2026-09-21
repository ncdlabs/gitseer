import { useEffect, useState } from "react";

export const DASHBOARD_RANGE_OPTIONS = [0, 1, 7, 30, 90] as const;
export type DashboardRangeDays = (typeof DASHBOARD_RANGE_OPTIONS)[number];

const STORAGE_KEY = "lens-dashboard-range-days";
export const DEFAULT_DASHBOARD_RANGE_DAYS: DashboardRangeDays = 0;

function isRangeDays(n: number): n is DashboardRangeDays {
  return (DASHBOARD_RANGE_OPTIONS as readonly number[]).includes(n);
}

function readStored(): DashboardRangeDays {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return DEFAULT_DASHBOARD_RANGE_DAYS;
  const n = Number(raw);
  return isRangeDays(n) ? n : DEFAULT_DASHBOARD_RANGE_DAYS;
}

export function useDashboardRange() {
  const [days, setDaysState] = useState<DashboardRangeDays>(readStored);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, String(days));
  }, [days]);

  function setDays(next: DashboardRangeDays) {
    setDaysState(next);
  }

  return { days, setDays };
}
