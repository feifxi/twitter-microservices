import { expect, test } from "@playwright/test";

const KONG_URL = process.env.E2E_KONG_URL ?? "http://localhost:8000";

// One test for the whole flow — the steps are a single causal chain (post →
// outbox → Kafka → indexer/trending) and splitting them would require seeding
// state independently per test. Serial mode is enforced by workers: 1 in the
// Playwright config.
test("post → profile timeline → trending → search", async ({ page, request }) => {
	const tag = `#e2e${Date.now()}`;
	const body = `playwright e2e ${tag} ${Math.random().toString(36).slice(2, 8)}`;

	await page.goto("/home");
	await expect(page).toHaveURL(/\/home/);

	// Scope to <main> — the sidebar also has a "Post" button (compose shortcut).
	const main = page.getByRole("main");
	const composer = main.getByPlaceholder(/what.*happening/i);
	await composer.click();
	await composer.fill(body);
	await main.getByRole("button", { name: /^post$/i }).click();

	// Profile timeline reads tweet-service Postgres directly — synchronous, no
	// Kafka in the path. A fresh user with no follows has an empty home feed
	// (self-tweets don't fan-out to self), so we verify the write via /profile.
	const userId = await currentUserId(page);
	await page.goto(`/profile/${userId}`);
	await expect(main.getByText(body, { exact: false }).first()).toBeVisible({
		timeout: 30_000,
	});

	// Trending is Kafka-eventual (tweet.created → feed-service consumer → ZSET).
	// The endpoint is public (no auth) and limit=200 cuts past any pre-existing
	// top-N noise that would push our single-count tag out.
	await expect
		.poll(
			async () => {
				const res = await request.get(`${KONG_URL}/v1/feed/trending?limit=200`);
				if (!res.ok()) return [];
				const { trending } = (await res.json()) as { trending: Array<{ tag: string }> };
				return trending.map((t) => t.tag);
			},
			{ timeout: 60_000, intervals: [1_000, 2_000, 5_000] },
		)
		.toContain(tag);

	// Search is also Kafka-eventual (tweet.created → search-service indexer →
	// OpenSearch). The /explore page renders matches under the Tweets tab.
	await page.goto(`/explore?q=${encodeURIComponent(tag)}`);
	await expect
		.poll(
			async () => (await main.getByText(body, { exact: false }).count()) > 0,
			{ timeout: 60_000, intervals: [1_000, 2_000, 5_000] },
		)
		.toBe(true);
});

async function currentUserId(page: import("@playwright/test").Page): Promise<string> {
	const res = await page.request.get("/v1/users/me");
	if (!res.ok()) throw new Error(`get /v1/users/me: ${res.status()}`);
	const { id } = (await res.json()) as { id: string };
	return id;
}
