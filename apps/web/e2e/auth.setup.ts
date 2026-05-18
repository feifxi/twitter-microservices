import { request, test as setup } from "@playwright/test";
import path from "node:path";

const AUTH_FILE = path.join(__dirname, "..", "playwright", ".auth", "user.json");

// Keycloak URL must match the issuer Kong validates against
// (http://localhost:8080/realms/twitter) — see local/kong/kong.yaml.
const APP_URL = process.env.E2E_APP_URL ?? "http://localhost:3000";
const KEYCLOAK_URL = process.env.E2E_KEYCLOAK_URL ?? "http://localhost:8080";
const REALM = "twitter";
const APP_CLIENT_ID = "twitter-app";
const ADMIN_USER = process.env.E2E_KC_ADMIN ?? "admin";
const ADMIN_PASSWORD = process.env.E2E_KC_ADMIN_PASSWORD ?? "admin";

const USER_SERVICE_URL = process.env.E2E_USER_SERVICE_URL ?? "http://localhost:8001";
const KONG_URL = process.env.E2E_KONG_URL ?? "http://localhost:8000";
const SERVICE_TOKEN = process.env.SERVICE_TOKEN;

// Stable test identity. Same on every run — Keycloak + user-service creation
// is idempotent. The unique-per-run state (hashtag, body) lives in the spec.
const TEST_USERNAME = "playwright-e2e";
const TEST_EMAIL = "playwright-e2e@test.local";
const TEST_PASSWORD = "Playwright-E2E-Passw0rd!";
const TEST_DISPLAY_NAME = "Playwright E2E";

setup("authenticate test user", async () => {
	if (!SERVICE_TOKEN) {
		throw new Error(
			"SERVICE_TOKEN env var is required — run `make test-e2e` so .env is sourced.",
		);
	}

	const api = await request.newContext();

	const adminToken = await directGrant(api, "master", "admin-cli", ADMIN_USER, ADMIN_PASSWORD);
	await enableUnmanagedAttributes(api, adminToken);
	const userId = await upsertKeycloakUser(api, adminToken);
	await provisionInUserService(api, userId);
	const tokens = await directGrant(api, REALM, APP_CLIENT_ID, TEST_USERNAME, TEST_PASSWORD, true);
	await ensureUsername(api, tokens.access_token);

	// Mirror what the /api/auth/callback handler sets — same names, same flags.
	const appOrigin = new URL(APP_URL);
	const cookies = [
		cookie("access_token", tokens.access_token, appOrigin),
		cookie("refresh_token", tokens.refresh_token, appOrigin),
		cookie("id_token", tokens.id_token, appOrigin),
	];

	const ctx = await request.newContext();
	await ctx.storageState({ path: AUTH_FILE });
	// storageState writes an empty cookies array — patch it in. Playwright's
	// addCookies requires a browser context, so we hand-write the file.
	const fs = await import("node:fs/promises");
	const state = JSON.parse(await fs.readFile(AUTH_FILE, "utf8"));
	state.cookies = cookies;
	await fs.writeFile(AUTH_FILE, JSON.stringify(state, null, 2));
});

function cookie(name: string, value: string, origin: URL) {
	return {
		name,
		value,
		domain: origin.hostname,
		path: "/",
		httpOnly: true,
		secure: origin.protocol === "https:",
		sameSite: "Lax" as const,
		expires: -1,
	};
}

interface TokenSet {
	access_token: string;
	refresh_token: string;
	id_token: string;
}

async function directGrant(
	api: Awaited<ReturnType<typeof request.newContext>>,
	realm: string,
	clientId: string,
	username: string,
	password: string,
	withIdToken = false,
): Promise<TokenSet> {
	const params = new URLSearchParams({
		grant_type: "password",
		client_id: clientId,
		username,
		password,
	});
	if (withIdToken) params.set("scope", "openid");
	const res = await api.post(
		`${KEYCLOAK_URL}/realms/${realm}/protocol/openid-connect/token`,
		{
			headers: { "content-type": "application/x-www-form-urlencoded" },
			data: params.toString(),
		},
	);
	if (!res.ok()) {
		throw new Error(`direct grant ${realm}/${clientId}/${username}: ${res.status()} ${await res.text()}`);
	}
	return res.json();
}

// Keycloak 26 ships with declarative user profile enabled and unmanaged
// attributes blocked. The ProvisionUser SPI relies on a `provisioned`
// attribute to avoid re-firing the required action on every login — without
// this flip, the attribute silently no-ops and direct-grant always 400s with
// "Account is not fully set up". Idempotent.
async function enableUnmanagedAttributes(
	api: Awaited<ReturnType<typeof request.newContext>>,
	adminToken: TokenSet,
) {
	const headers = { authorization: `Bearer ${adminToken.access_token}` };
	const cur = await api.get(`${KEYCLOAK_URL}/admin/realms/${REALM}/users/profile`, { headers });
	if (!cur.ok()) {
		throw new Error(`get user profile config: ${cur.status()} ${await cur.text()}`);
	}
	const profile = (await cur.json()) as { unmanagedAttributePolicy?: string };
	if (profile.unmanagedAttributePolicy === "ENABLED") return;
	const put = await api.put(`${KEYCLOAK_URL}/admin/realms/${REALM}/users/profile`, {
		headers: { ...headers, "content-type": "application/json" },
		data: { ...profile, unmanagedAttributePolicy: "ENABLED" },
	});
	if (!put.ok()) {
		throw new Error(`enable unmanaged attributes: ${put.status()} ${await put.text()}`);
	}
}

async function upsertKeycloakUser(
	api: Awaited<ReturnType<typeof request.newContext>>,
	adminToken: TokenSet,
): Promise<string> {
	const headers = { authorization: `Bearer ${adminToken.access_token}` };
	const lookup = await api.get(
		`${KEYCLOAK_URL}/admin/realms/${REALM}/users?username=${TEST_USERNAME}&exact=true`,
		{ headers },
	);
	if (!lookup.ok()) {
		throw new Error(`lookup user: ${lookup.status()} ${await lookup.text()}`);
	}
	const existing = (await lookup.json()) as Array<{ id: string }>;

	let id: string;
	if (existing.length > 0) {
		id = existing[0].id;
	} else {
		const create = await api.post(`${KEYCLOAK_URL}/admin/realms/${REALM}/users`, {
			headers: { ...headers, "content-type": "application/json" },
			data: {
				username: TEST_USERNAME,
				email: TEST_EMAIL,
				firstName: "Playwright",
				lastName: "E2E",
				enabled: true,
				emailVerified: true,
				credentials: [{ type: "password", value: TEST_PASSWORD, temporary: false }],
			},
		});
		if (!create.ok() && create.status() !== 409) {
			throw new Error(`create user: ${create.status()} ${await create.text()}`);
		}
		const after = await api.get(
			`${KEYCLOAK_URL}/admin/realms/${REALM}/users?username=${TEST_USERNAME}&exact=true`,
			{ headers },
		);
		const users = (await after.json()) as Array<{ id: string }>;
		if (users.length === 0) throw new Error("user not found after create");
		id = users[0].id;
	}

	// Always clear required actions and set the provisioned attribute — the
	// ProvisionUser SPI's evaluateTriggers re-adds PROVISION_USER on every
	// auth unless attributes.provisioned == "true". Our setup POSTs to
	// /internal/provision directly, so the interactive challenge is unwanted.
	const update = await api.put(`${KEYCLOAK_URL}/admin/realms/${REALM}/users/${id}`, {
		headers: { ...headers, "content-type": "application/json" },
		data: {
			username: TEST_USERNAME,
			email: TEST_EMAIL,
			firstName: "Playwright",
			lastName: "E2E",
			requiredActions: [],
			emailVerified: true,
			enabled: true,
			attributes: { provisioned: ["true"] },
		},
	});
	if (!update.ok()) {
		throw new Error(`clear required actions: ${update.status()} ${await update.text()}`);
	}
	return id;
}

// Sets a username via Kong so the (main) layout doesn't redirect us to
// /onboarding. Idempotent — repeated PATCHes are no-ops once username is set.
async function ensureUsername(
	api: Awaited<ReturnType<typeof request.newContext>>,
	accessToken: string,
) {
	const me = await api.get(`${KONG_URL}/v1/users/me`, {
		headers: { authorization: `Bearer ${accessToken}` },
	});
	if (!me.ok()) {
		throw new Error(`get /v1/users/me: ${me.status()} ${await me.text()}`);
	}
	const profile = (await me.json()) as { username: string | null };
	if (profile.username) return;
	const patch = await api.patch(`${KONG_URL}/v1/users/me`, {
		headers: {
			authorization: `Bearer ${accessToken}`,
			"content-type": "application/json",
		},
		data: { username: TEST_USERNAME },
	});
	if (!patch.ok()) {
		throw new Error(`patch /v1/users/me: ${patch.status()} ${await patch.text()}`);
	}
}

async function provisionInUserService(
	api: Awaited<ReturnType<typeof request.newContext>>,
	keycloakSub: string,
) {
	const res = await api.post(`${USER_SERVICE_URL}/internal/provision`, {
		headers: {
			"content-type": "application/json",
			authorization: `Bearer ${SERVICE_TOKEN}`,
		},
		data: {
			keycloak_sub: keycloakSub,
			email: TEST_EMAIL,
			display_name: TEST_DISPLAY_NAME,
		},
	});
	// 200 ok, 201 created — both fine; idempotent.
	if (res.status() !== 200 && res.status() !== 201) {
		throw new Error(`provision user: ${res.status()} ${await res.text()}`);
	}
}
