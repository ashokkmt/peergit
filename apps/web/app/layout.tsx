import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "PeerGit — campus projects",
  description: "A shared home for campus projects and collaboration.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className="bg-canvas text-on-surface scroll-smooth overflow-x-clip antialiased"
    >
      <head>
        <script src="/appearance.js" />
      </head>
      <body className="m-0 min-h-screen flex flex-col bg-canvas text-on-surface font-sans text-sm leading-[21px]">
        {children}
      </body>
    </html>
  );
}
