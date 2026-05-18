import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { NextRequest } from "next/server";
import {
	APP_URL,
	KEYCLOAK_CLIENT_ID,
	KEYCLOAK_REALM,
	KEYCLOAK_URL,
} from "@/lib/config";

const REDIRECT_URI = `${APP_URL}/api/auth/callback`;

export async function GET(request: NextRequest) {
	const { searchParams } = request.nextUrl;
	const code = searchParams.get("code");
	const error = searchParams.get("error");

	if (error || !code) {
		return redirect(
			`/login?error=${encodeURIComponent(error ?? "missing_code")}`,
		);
	}

	const tokenRes = await fetch(
		`${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/token`,
		{
			method: "POST",
			headers: { "Content-Type": "application/x-www-form-urlencoded" },
			body: new URLSearchParams({
				grant_type: "authorization_code",
				client_id: KEYCLOAK_CLIENT_ID,
				redirect_uri: REDIRECT_URI,
				code,
			}),
		},
	);

	if (!tokenRes.ok) {
		return redirect("/login?error=token_exchange_failed");
	}

	const tokenData = await tokenRes.json();
	const cookieStore = await cookies();

	cookieStore.set("access_token", tokenData.access_token, {
		httpOnly: true,
		secure: process.env.NODE_ENV === "production",
		sameSite: "lax",
		maxAge: tokenData.expires_in ?? 900,
		path: "/",
	});

	if (tokenData.refresh_token) {
		cookieStore.set("refresh_token", tokenData.refresh_token, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: tokenData.refresh_expires_in ?? 36000,
			path: "/",
		});
	}

	if (tokenData.id_token) {
		cookieStore.set("id_token", tokenData.id_token, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: tokenData.expires_in ?? 900,
			path: "/",
		});
	}

	redirect("/home");
}
