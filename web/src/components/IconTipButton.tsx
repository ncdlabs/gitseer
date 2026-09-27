import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Glyph, type ShellIconName } from "./Glyph";

export type IconTipButtonProps = {
  label: string;
  icon: ShellIconName;
  danger?: boolean;
  compact?: boolean;
  className?: string;
  children?: ReactNode;
} & Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children" | "aria-label" | "className">;

/** Icon-only control with an instant CSS tooltip (no native title delay). */
export function IconTipButton({
  label,
  icon,
  danger,
  compact,
  className,
  type = "button",
  ...rest
}: IconTipButtonProps) {
  const classes = [
    "icon-tip-btn",
    danger ? "icon-tip-btn--danger" : "",
    compact ? "icon-tip-btn--compact" : "",
    className || "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button className={classes} type={type} aria-label={label} {...rest}>
      <Glyph name={icon} />
      <span className="icon-tip-btn__tip" role="tooltip">
        {label}
      </span>
    </button>
  );
}
