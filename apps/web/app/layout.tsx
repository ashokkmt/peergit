import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "PeerGit — campus projects",
  description: "A shared home for campus projects and collaboration.",
  icons: {
    icon: [
      { url: "/icons/favicon-light.ico", sizes: "any", media: "(prefers-color-scheme: light)" },
      { url: "/icons/favicon-dark.ico", sizes: "any", media: "(prefers-color-scheme: dark)" },
      { url: "/icons/icon-light.svg", type: "image/svg+xml", media: "(prefers-color-scheme: light)" },
      { url: "/icons/icon-dark.svg", type: "image/svg+xml", media: "(prefers-color-scheme: dark)" },
    ],
    apple: [
      { url: "/icons/apple-icon-light.png", media: "(prefers-color-scheme: light)" },
      { url: "/icons/apple-icon-dark.png", media: "(prefers-color-scheme: dark)" },
    ],
  },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><head><link rel="manifest" href="/manifest-light.json" media="(prefers-color-scheme: light)" /><link rel="manifest" href="/manifest-dark.json" media="(prefers-color-scheme: dark)" /></head><body>{children}</body></html>;
}
