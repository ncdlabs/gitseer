import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "gruvbox" | "system";

export type ThemeOption = {
  id: Theme;
  label: string;
};

export const THEME_OPTIONS: ThemeOption[] = [
  { id: "system", label: "System" },
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
  { id: "gruvbox", label: "Gruvbox" },
];

const THEME_KEY = "lens-theme";
const MANUAL_KEY = "lens-theme-manual";

function isTheme(v: string | null | undefined): v is Theme {
  return v === "light" || v === "dark" || v === "gruvbox" || v === "system";
}

function resolveTheme(theme: Theme, prefersDark: boolean): "light" | "dark" | "gruvbox" {
  if (theme === "system") return prefersDark ? "dark" : "light";
  return theme;
}

function readStoredTheme(): Theme | null {
  const stored = localStorage.getItem(THEME_KEY);
  return isTheme(stored) ? stored : null;
}

function isManualOverride(): boolean {
  return localStorage.getItem(MANUAL_KEY) === "1";
}

/**
 * Theme preference: if the user has not picked a Lens theme manually, apply a
 * mapped Gitea default (light/dark/auto). Custom Gitea themes fail closed —
 * leave Lens on system / local choice.
 */
export function useTheme(giteaTheme?: Theme | null) {
  const [theme, setThemeState] = useState<Theme>(() => {
    if (isManualOverride()) {
      return readStoredTheme() ?? "system";
    }
    if (isTheme(giteaTheme)) return giteaTheme;
    return readStoredTheme() ?? "system";
  });

  useEffect(() => {
    if (isManualOverride()) return;
    if (isTheme(giteaTheme)) {
      setThemeState(giteaTheme);
    }
  }, [giteaTheme]);

  useEffect(() => {
    const root = document.documentElement;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      root.setAttribute("data-theme", resolveTheme(theme, mq.matches));
    };
    apply();
    localStorage.setItem(THEME_KEY, theme);
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [theme]);

  function setTheme(next: Theme) {
    localStorage.setItem(MANUAL_KEY, "1");
    setThemeState(next);
  }

  return { theme, setTheme };
}
