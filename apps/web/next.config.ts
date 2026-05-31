import type { NextConfig } from "next";

const KONG_URL = process.env.KONG_URL ?? "http://localhost:8000";

const nextConfig: NextConfig = {
	// Required for the Docker image to ship a minimal runtime tree.
	output: "standalone",
	...(process.env.NODE_ENV === "development" && {
		async rewrites() {
			return [{ source: "/v1/:path*", destination: `${KONG_URL}/v1/:path*` }];
		},
	}),
};

export default nextConfig;
