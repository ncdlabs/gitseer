import { brandSrc, type BrandVariant } from "../lib/branding";
import { useResolvedTheme } from "../hooks/useResolvedTheme";

type Props = {
  variant?: BrandVariant;
  className?: string;
  alt?: string;
};

/** Theme-aware official Gitea Lens logo (lockup) or mark (emblem). */
export function Brand({ variant = "logo", className, alt = "Gitea Lens" }: Props) {
  const theme = useResolvedTheme();
  return (
    <img
      className={className ? `brand-img ${className}` : "brand-img"}
      src={brandSrc(theme, variant)}
      alt={alt}
      draggable={false}
    />
  );
}
