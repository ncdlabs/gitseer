import { forgeLabel } from "../api/client";

type Props = {
  forgeType?: string | null;
  /** Optional instance display name; shown when distinct from the forge label. */
  instanceName?: string | null;
  className?: string;
};

/** Small forge badge for list cards and table rows. */
export function ForgeBadge({ forgeType, instanceName, className }: Props) {
  const label = forgeLabel(forgeType || "gitea");
  const kind = (forgeType || "gitea").toLowerCase();
  const inst = (instanceName || "").trim();
  const showInst =
    !!inst &&
    inst.toLowerCase() !== label.toLowerCase() &&
    inst.toLowerCase() !== kind;

  return (
    <span className={`forge-badges${className ? ` ${className}` : ""}`}>
      <span className={`badge badge--forge badge--forge-${kind}`} title={label}>
        {label}
      </span>
      {showInst && (
        <span className="badge badge--forge-instance" title={inst}>
          {inst}
        </span>
      )}
    </span>
  );
}
