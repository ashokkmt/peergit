import type { NextConfig } from "next";

const config: NextConfig = {
  async rewrites() {
    const rewrites = [{ source: "/readyz", destination: `${process.env.API_ORIGIN ?? "http://127.0.0.1:8080"}/readyz` }];
    if (process.env.API_PROXY_ORIGIN) {
      rewrites.push({ source: "/api/:path*", destination: `${process.env.API_PROXY_ORIGIN}/api/:path*` });
    }
    return rewrites;
  },
};

export default config;
