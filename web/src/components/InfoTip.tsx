import type { MouseEvent, ReactNode } from "react";
import { Glyph } from "./Glyph";

type Props = {
  label: string;
  children: ReactNode;
};

export function InfoTip({ label, children }: Props) {
  function stopLabelActivation(e: MouseEvent<HTMLButtonElement>) {
    e.preventDefault();
    e.stopPropagation();
  }

  return (
    <button
      type="button"
      className="info-tip"
      tabIndex={-1}
      aria-label={label}
      onClick={stopLabelActivation}
      onMouseDown={stopLabelActivation}
    >
      <Glyph name="info" className="info-tip__icon" />
      <span className="info-tip__bubble" role="tooltip">
        {children}
      </span>
    </button>
  );
}
