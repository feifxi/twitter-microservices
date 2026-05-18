import { defineConfig, devices } from "@playwright/test";

const APP_URL = process.env.E2E_APP_URL ?? "http://localhost:3000";

export default defineConfig({
	testDir: "./e2e",
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	// Kafka fan-out + 30s frontend refetch + OpenSearch indexing each consume
	// part of the budget; 180s is comfortable but still fails fast on real bugs.
	timeout: 180_000,
	reporter: process.env.CI ? "github" : "list",
	use: {
		baseURL: APP_URL,
		trace: "retain-on-failure",
		screenshot: "only-on-failure",
		video: "retain-on-failure",
	},
	projects: [
		{
			name: "setup",
			testMatch: /.*\.setup\.ts$/,
		},
		{
			name: "chromium",
			use: {
				...devices["Desktop Chrome"],
				storageState: "playwright/.auth/user.json",
			},
			dependencies: ["setup"],
		},
	],
	webServer: {
		command: "npm run dev",
		url: APP_URL,
		reuseExistingServer: true,
		timeout: 60_000,
	},
});
