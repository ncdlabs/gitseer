/** Resolved `data-theme` value (never `system`). */
export type ResolvedTheme = "light" | "dark" | "gruvbox" | "terminal";

export type BrandVariant = "logo" | "mark";

export const brandColors: Record<ResolvedTheme, { base: string; accent: string }> = {
  light: { base: "#181C21", accent: "#4183C4" },
  dark: { base: "#D0D5DA", accent: "#4183C4" },
  gruvbox: { base: "#EBDBB2", accent: "#FE8019" },
  terminal: { base: "#B8F0B8", accent: "#39FF14" },
};

export function isResolvedTheme(value: string | null | undefined): value is ResolvedTheme {
  return value === "light" || value === "dark" || value === "gruvbox" || value === "terminal";
}
