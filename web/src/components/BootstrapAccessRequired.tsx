type Props = {
  /** What this tab needs bootstrap for (sentence before the link). */
  message: string;
  canElevate?: boolean;
  onBecomeBootstrap?: () => void;
};

/** Content-area gate when a Settings tab needs bootstrap admin (or a 5-minute elevation). */
export function BootstrapAccessRequired({ message, canElevate, onBecomeBootstrap }: Props) {
  return (
    <div className="panel panel--padded">
      <p className="muted">
        {message}
        {canElevate && onBecomeBootstrap ? (
          <>
            {" "}
            <button type="button" className="linkish" onClick={onBecomeBootstrap}>
              Become Bootstrap
            </button>{" "}
            for a 5-minute grant.
          </>
        ) : null}
      </p>
    </div>
  );
}
