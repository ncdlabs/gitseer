import { Glyph } from "./Glyph";

type Props = {
  onExpandAll: () => void;
  onCollapseAll: () => void;
  canExpandAll?: boolean;
  canCollapseAll?: boolean;
};

export function ExpandCollapseControls({
  onExpandAll,
  onCollapseAll,
  canExpandAll = true,
  canCollapseAll = true,
}: Props) {
  return (
    <div className="expand-collapse-controls" role="group" aria-label="Expand or collapse">
      <button
        type="button"
        className="expand-collapse-controls__btn"
        aria-label="Expand All"
        title="Expand All"
        disabled={!canExpandAll}
        onClick={onExpandAll}
      >
        <Glyph name="expandAll" />
      </button>
      <button
        type="button"
        className="expand-collapse-controls__btn"
        aria-label="Collapse All"
        title="Collapse All"
        disabled={!canCollapseAll}
        onClick={onCollapseAll}
      >
        <Glyph name="collapseAll" />
      </button>
    </div>
  );
}
