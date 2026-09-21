import logoDay from "../assets/branding/gitea-lens-logo-day.png";
import logoGruvbox from "../assets/branding/gitea-lens-logo-gruvbox.png";
import logoNight from "../assets/branding/gitea-lens-logo-night.png";
import logoTerminal from "../assets/branding/gitea-lens-logo-terminal-green.png";
import markDay from "../assets/branding/gitea-lens-mark-day.png";
import markGruvbox from "../assets/branding/gitea-lens-mark-gruvbox.png";
import markNight from "../assets/branding/gitea-lens-mark-night.png";
import markTerminal from "../assets/branding/gitea-lens-mark-terminal-green.png";

/** Resolved `data-theme` value (never `system`). */
export type ResolvedTheme = "light" | "dark" | "gruvbox" | "terminal";

export type BrandVariant = "logo" | "mark";

const logos: Record<ResolvedTheme, string> = {
  light: logoDay,
  dark: logoNight,
  gruvbox: logoGruvbox,
  terminal: logoTerminal,
};

const marks: Record<ResolvedTheme, string> = {
  light: markDay,
  dark: markNight,
  gruvbox: markGruvbox,
  terminal: markTerminal,
};

export function isResolvedTheme(value: string | null | undefined): value is ResolvedTheme {
  return value === "light" || value === "dark" || value === "gruvbox" || value === "terminal";
}

export function brandSrc(theme: ResolvedTheme, variant: BrandVariant): string {
  return variant === "logo" ? logos[theme] : marks[theme];
}
