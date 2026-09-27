import { Link } from "react-router-dom";
import { hrefForVisit, labelForVisit, useVisitHistory } from "../hooks/useVisitHistory";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";
import { Glyph } from "./Glyph";

const MAX_PREVIOUS = 5;

export function VisitBreadcrumbs() {
  const previous = useVisitHistory(MAX_PREVIOUS);
  const terminal = useIsTerminalTheme();

  if (previous.length === 0) return null;

  return (
    <nav className="visit-breadcrumbs" aria-label="Recent pages">
      <ol className="visit-breadcrumbs__list">
        {previous.map((entry, index) => {
          const href = hrefForVisit(entry);
          const label = labelForVisit(entry);
          return (
            <li key={`${href}-${index}`} className="visit-breadcrumbs__item">
              {index > 0 && (
                <span className="visit-breadcrumbs__sep" aria-hidden="true">
                  {terminal ? ">" : <Glyph name="chevron" />}
                </span>
              )}
              <Link className="visit-breadcrumbs__link" to={href} title={label}>
                {label}
              </Link>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
