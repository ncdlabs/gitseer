import { useEffect, useState } from "react";

export type ViewMode = "table" | "cards";

const STORAGE_KEY = "lens-view-mode";

function readStored(): ViewMode {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === "cards" || stored === "table" ? stored : "table";
}

export function useViewMode() {
  const [mode, setMode] = useState<ViewMode>(readStored);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, mode);
  }, [mode]);

  return { mode, setMode };
}
