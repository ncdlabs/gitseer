import type { MouseEvent, ReactNode } from "react";

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
      <svg className="info-tip__icon" viewBox="0 0 16 16" aria-hidden="true">
        <circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" strokeWidth="1.25" />
        <path
          d="M8 7.25v3.5M8 5.25h.01"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
      </svg>
      <span className="info-tip__bubble" role="tooltip">
        {children}
      </span>
    </button>
  );
}
