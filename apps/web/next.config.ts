import type { NextConfig } from "next";

const config: NextConfig = {
  async rewrites() {
    return [{ source: "/readyz", destination: `${process.env.API_ORIGIN ?? "http://127.0.0.1:8080"}/readyz` }];
  },
};

export default config;
