import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { KEYCLOAK_CLIENT_ID, KEYCLOAK_REALM, KEYCLOAK_URL } from "@/lib/config";

// POST /api/auth/refresh — called by the Axios response interceptor when a
// /v1/* request returns 401. Uses the httpOnly refresh_token cookie to mint
// a new access token from Keycloak and writes both rotated tokens back as
// httpOnly cookies. The interceptor then retries the original request, which
// the next middleware pass will forward with the fresh Authorization header.
//
// Returns 401 if refresh fails (no refresh cookie, Keycloak rejects). The
// caller is responsible for redirecting the user to /login in that case.
export async function POST() {
	const cookieStore = await cookies();
	const refreshToken = cookieStore.get("refresh_token")?.value;
	if (!refreshToken) {
		return new NextResponse(null, { status: 401 });
	}

	const res = await fetch(
		`${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/token`,
		{
			method: "POST",
			headers: { "Content-Type": "application/x-www-form-urlencoded" },
			body: new URLSearchParams({
				grant_type: "refresh_token",
				client_id: KEYCLOAK_CLIENT_ID,
				refresh_token: refreshToken,
			}),
		},
	);

	if (!res.ok) {
		cookieStore.delete("access_token");
		cookieStore.delete("refresh_token");
		cookieStore.delete("id_token");
		return new NextResponse(null, { status: 401 });
	}

	const tokens = (await res.json()) as {
		access_token: string;
		refresh_token: string;
		expires_in: number;
		refresh_expires_in: number;
	};

	const secure = process.env.NODE_ENV === "production";
	cookieStore.set("access_token", tokens.access_token, {
		httpOnly: true,
		secure,
		sameSite: "lax",
		maxAge: tokens.expires_in ?? 900,
		path: "/",
	});
	cookieStore.set("refresh_token", tokens.refresh_token, {
		httpOnly: true,
		secure,
		sameSite: "lax",
		maxAge: tokens.refresh_expires_in ?? 36000,
		path: "/",
	});

	return new NextResponse(null, { status: 200 });
}
