import type { CSSProperties } from "react";
import { brandColors, type BrandVariant } from "../lib/branding";
import { useResolvedTheme } from "../hooks/useResolvedTheme";

type Props = { variant?: BrandVariant; className?: string; alt?: string };

/** Vector emblem with a live, locally hosted Inter wordmark. */
export function Brand({ variant = "logo", className, alt = "GitSeer" }: Props) {
  const theme = useResolvedTheme();
  const { base, accent } = brandColors[theme];
  return (
    <span
      className={`brand-img gitseer-brand gitseer-brand--${variant}${className ? ` ${className}` : ""}`}
      style={{ "--brand-base": base, "--brand-accent": accent } as CSSProperties}
      role={alt ? "img" : undefined}
      aria-label={alt || undefined}
      aria-hidden={alt ? undefined : true}
    >
      <svg className="gitseer-brand__emblem" viewBox="0 0 200 120" fill="none" aria-hidden="true" focusable="false">
        <g fill="var(--brand-base)">
          <path fillRule="evenodd" d="M0 60C32 22 65 0 100 0s68 22 100 60c-32 38-65 60-100 60S32 98 0 60Zm20 0c28 30 54 46 80 46s52-16 80-46c-28-30-54-46-80-46S48 30 20 60Z" />
          <path fillRule="evenodd" d="M100 0a60 60 0 1 1 0 120 60 60 0 0 1 0-120Zm0 14a46 46 0 1 0 0 92 46 46 0 0 0 0-92Z" />
        </g>
        <g fill="var(--brand-accent)" stroke="var(--brand-accent)" strokeWidth="10" strokeLinecap="round" strokeLinejoin="round">
          <path d="M100 35v13c0 10-12 23-22 31m22-31c0 10 12 23 22 31" fill="none" />
          <circle cx="100" cy="35" r="12" stroke="none" />
          <circle cx="78" cy="79" r="12" stroke="none" />
          <circle cx="122" cy="79" r="12" stroke="none" />
        </g>
      </svg>
      {variant === "logo" && <span className="gitseer-brand__wordmark" aria-hidden="true">Git<span>Seer</span></span>}
    </span>
  );
}
