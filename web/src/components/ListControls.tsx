import type { ReactNode } from "react";
import type { ViewMode } from "../hooks/useViewMode";
import { ListFilter } from "./ListFilter";
import { ViewModeToggle } from "./ViewModeToggle";

type Props = {
  mode: ViewMode;
  onMode: (mode: ViewMode) => void;
  filter: string;
  onFilter: (query: string) => void;
  filterPlaceholder?: string;
  filterLabel?: string;
  children?: ReactNode;
};

export function ListControls({
  mode,
  onMode,
  filter,
  onFilter,
  filterPlaceholder,
  filterLabel,
  children,
}: Props) {
  return (
    <div className="list-controls">
      {children}
      <ListFilter
        value={filter}
        onApply={onFilter}
        placeholder={filterPlaceholder}
        label={filterLabel}
      />
      <ViewModeToggle mode={mode} onMode={onMode} />
    </div>
  );
}
