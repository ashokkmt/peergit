export function PreviewBadge({ phase }: { phase?: string }) {
  return (
    <span className="prototype-note">
      Will get implemented in a later phase{phase ? ` · ${phase}` : ""}
    </span>
  );
}
