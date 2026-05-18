import type { APIResponse } from "@playwright/test";
import { test as setup } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

const KEYCLOAK = "http://localhost:8080";
const KONG = "http://localhost:8000";
const USER_SVC = "http://localhost:8001";
const REALM = "twitter";
const CLIENT_ID = "twitter-app";

const TEST = {
	username: "playwright-e2e",
	email: "playwright-e2e@test.local",
	password: "Playwright-E2E-Passw0rd!",
	displayName: "Playwright E2E",
};

const AUTH_FILE = path.join(__dirname, "..", "playwright", ".auth", "user.json");

setup("authenticate test user", async ({ request: api }) => {
	const serviceToken = process.env.SERVICE_TOKEN;
	if (!serviceToken) throw new Error("SERVICE_TOKEN required — run via `make test-e2e`.");

	// 1. Admin token via direct grant against master.
	const admin = await grant("master", "admin-cli", "admin", "admin");
	const auth = { authorization: `Bearer ${admin.access_token}` };
	const json = { ...auth, "content-type": "application/json" };

	// 2. Allow unmanaged user attributes. Keycloak 26 blocks them by default;
	// the ProvisionUser SPI's `provisioned` attribute is how we mark the
	// account set up so direct-grant doesn't 400 with "Account is not fully
	// set up" on every login.
	await mustOk(api.put(`${KEYCLOAK}/admin/realms/${REALM}/users/profile`, {
		headers: json, data: { unmanagedAttributePolicy: "ENABLED" },
	}), "enable unmanaged attrs");

	// 3. Upsert test user, then patch attrs + clear required actions.
	let userId = await findUserId();
	if (!userId) {
		await createUser();
		userId = await findUserId();
		if (!userId) throw new Error("user not found after create");
	}
	await mustOk(api.put(`${KEYCLOAK}/admin/realms/${REALM}/users/${userId}`, {
		headers: json,
		data: {
			username: TEST.username, email: TEST.email,
			firstName: "Playwright", lastName: "E2E",
			enabled: true, emailVerified: true,
			requiredActions: [],
			attributes: { provisioned: ["true"] },
		},
	}), "patch user");

	// 4. Provision the row in user-service. Interactive logins do this via the
	// SPI; direct-grant bypasses required actions, so we POST ourselves.
	await mustOk(api.post(`${USER_SVC}/internal/provision`, {
		headers: { "content-type": "application/json", authorization: `Bearer ${serviceToken}` },
		data: { keycloak_sub: userId, email: TEST.email, display_name: TEST.displayName },
	}), "provision user", [200, 201]);

	// 5. User token; PATCH username so (main) layout doesn't redirect to /onboarding.
	const user = await grant(REALM, CLIENT_ID, TEST.username, TEST.password, true);
	const bearer = { authorization: `Bearer ${user.access_token}` };
	const me = await mustJson<{ username: string | null }>(
		api.get(`${KONG}/v1/users/me`, { headers: bearer }), "GET /v1/users/me");
	if (!me.username) {
		await mustOk(api.patch(`${KONG}/v1/users/me`, {
			headers: { ...bearer, "content-type": "application/json" },
			data: { username: TEST.username },
		}), "PATCH /v1/users/me");
	}

	// 6. Write storageState directly. Cookie names match what /api/auth/callback sets.
	await mkdir(path.dirname(AUTH_FILE), { recursive: true });
	const c = { domain: "localhost", path: "/", httpOnly: true, secure: false, sameSite: "Lax" as const, expires: -1 };
	await writeFile(AUTH_FILE, JSON.stringify({
		cookies: [
			{ ...c, name: "access_token", value: user.access_token },
			{ ...c, name: "refresh_token", value: user.refresh_token },
			{ ...c, name: "id_token", value: user.id_token },
		],
		origins: [],
	}));

	async function findUserId(): Promise<string | undefined> {
		const list = await mustJson<Array<{ id: string }>>(
			api.get(`${KEYCLOAK}/admin/realms/${REALM}/users?username=${TEST.username}&exact=true`, { headers: auth }),
			"lookup user");
		return list[0]?.id;
	}

	async function createUser() {
		await mustOk(api.post(`${KEYCLOAK}/admin/realms/${REALM}/users`, {
			headers: json,
			data: {
				username: TEST.username, email: TEST.email,
				firstName: "Playwright", lastName: "E2E",
				enabled: true, emailVerified: true,
				credentials: [{ type: "password", value: TEST.password, temporary: false }],
			},
		}), "create user", [409]);
	}

	async function grant(realm: string, clientId: string, username: string, password: string, openid = false) {
		const params = new URLSearchParams({ grant_type: "password", client_id: clientId, username, password });
		if (openid) params.set("scope", "openid");
		return mustJson<{ access_token: string; refresh_token: string; id_token: string }>(
			api.post(`${KEYCLOAK}/realms/${realm}/protocol/openid-connect/token`, {
				headers: { "content-type": "application/x-www-form-urlencoded" },
				data: params.toString(),
			}),
			`grant ${realm}/${clientId}/${username}`);
	}
});

async function mustOk(p: Promise<APIResponse>, label: string, allow: number[] = []) {
	const r = await p;
	if (r.ok() || allow.includes(r.status())) return r;
	throw new Error(`${label}: ${r.status()} ${await r.text()}`);
}

async function mustJson<T>(p: Promise<APIResponse>, label: string): Promise<T> {
	return (await mustOk(p, label)).json();
}
