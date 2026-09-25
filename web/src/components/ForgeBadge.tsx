import { forgeLabel } from "../api/client";

type Props = {
  forgeType?: string | null;
  className?: string;
};

/** Small forge badge for list cards and table rows. */
export function ForgeBadge({ forgeType, className }: Props) {
  const label = forgeLabel(forgeType || "gitea");
  const kind = (forgeType || "gitea").toLowerCase();
  return (
    <span
      className={`badge badge--forge badge--forge-${kind}${className ? ` ${className}` : ""}`}
      title={label}
    >
      {label}
    </span>
  );
}
