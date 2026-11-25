import { CampusConsoleShell } from "@/components/campus/campus-console-shell";

export default function CampusLayout({ children }: { children: React.ReactNode }) {
  return (
    <CampusConsoleShell
      title="Northgate State University"
      subtitle="Fall 2026 · Sample preview"
    >
      {children}
    </CampusConsoleShell>
  );
}
