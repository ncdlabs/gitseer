import type { ReactNode } from "react";
import type { ViewMode } from "../hooks/useViewMode";
import { ListFilter } from "./ListFilter";
import { ViewModeToggle } from "./ViewModeToggle";

type Props = {
  mode: ViewMode;
  onMode: (mode: ViewMode) => void;
  filter?: string;
  onFilter?: (query: string) => void;
  filterPlaceholder?: string;
  filterLabel?: string;
  /** When set, replaces the default ListFilter control. */
  filterControl?: ReactNode;
  children?: ReactNode;
};

export function ListControls({
  mode,
  onMode,
  filter = "",
  onFilter,
  filterPlaceholder,
  filterLabel,
  filterControl,
  children,
}: Props) {
  return (
    <div className="list-controls">
      {filterControl ?? (
        <ListFilter
          value={filter}
          onApply={onFilter ?? (() => {})}
          placeholder={filterPlaceholder}
          label={filterLabel}
        />
      )}
      {children}
      <ViewModeToggle mode={mode} onMode={onMode} />
    </div>
  );
}
