import { useEffect, useState } from "react";
import { isResolvedTheme, type ResolvedTheme } from "../lib/branding";

function readResolvedTheme(): ResolvedTheme {
  const value = document.documentElement.getAttribute("data-theme");
  return isResolvedTheme(value) ? value : "dark";
}

/** Current resolved theme from `<html data-theme>` (system already applied). */
export function useResolvedTheme(): ResolvedTheme {
  const [theme, setTheme] = useState(readResolvedTheme);

  useEffect(() => {
    const root = document.documentElement;
    const sync = () => setTheme(readResolvedTheme());
    sync();
    const mo = new MutationObserver(sync);
    mo.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    return () => mo.disconnect();
  }, []);

  return theme;
}
