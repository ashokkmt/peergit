import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "PeerGit — campus projects",
  description: "A shared home for campus projects and collaboration.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
