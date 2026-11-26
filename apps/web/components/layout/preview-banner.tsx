export function PreviewBadge({ phase }: { phase?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full border border-warning/45 bg-warning/10 text-warning text-xs font-semibold leading-4">
      Will get implemented in a later phase{phase ? ` · ${phase}` : ""}
    </span>
  );
}
