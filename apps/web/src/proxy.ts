import { type NextRequest, NextResponse } from "next/server";
import { KEYCLOAK_CLIENT_ID, KEYCLOAK_REALM, KEYCLOAK_URL } from "@/lib/config";
import { jwtExp } from "@/lib/jwt";

const REFRESH_THRESHOLD_SECONDS = 60;

async function refreshTokens(refreshToken: string) {
	try {
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
		if (!res.ok) return null;
		return res.json() as Promise<{
			access_token: string;
			refresh_token: string;
			expires_in: number;
			refresh_expires_in: number;
		}>;
	} catch {
		return null;
	}
}

// forwardWithAuth attaches Authorization: Bearer ... onto requests that the
// Next.js dev rewrite forwards to Kong, so Kong sees the token even though the
// access_token cookie is httpOnly and the browser can't attach it manually.
function forwardWithAuth(
	request: NextRequest,
	accessToken: string | undefined,
): NextResponse {
	if (!accessToken) return NextResponse.next();
	const headers = new Headers(request.headers);
	headers.set("Authorization", `Bearer ${accessToken}`);
	return NextResponse.next({ request: { headers } });
}

export async function proxy(request: NextRequest) {
	const { pathname } = request.nextUrl;

	if (pathname.startsWith("/api/auth") || pathname === "/login") {
		return NextResponse.next();
	}

	const accessToken = request.cookies.get("access_token")?.value;
	const refreshToken = request.cookies.get("refresh_token")?.value;

	const isApi = pathname.startsWith("/v1/");

	if (!accessToken && !refreshToken) {
		if (isApi) {
			return new NextResponse(null, { status: 401 });
		}
		return NextResponse.redirect(new URL("/login", request.url));
	}

	const now = Math.floor(Date.now() / 1000);
	const exp = accessToken ? jwtExp(accessToken) : null;
	const needsRefresh =
		!accessToken || (exp !== null && exp - now < REFRESH_THRESHOLD_SECONDS);

	if (!needsRefresh) return forwardWithAuth(request, accessToken);

	if (!refreshToken) {
		if (isApi) return new NextResponse(null, { status: 401 });
		return NextResponse.redirect(new URL("/login", request.url));
	}

	const tokens = await refreshTokens(refreshToken);
	if (!tokens) {
		if (isApi) {
			const response = new NextResponse(null, { status: 401 });
			response.cookies.delete("access_token");
			response.cookies.delete("refresh_token");
			return response;
		}
		const response = NextResponse.redirect(new URL("/login", request.url));
		response.cookies.delete("access_token");
		response.cookies.delete("refresh_token");
		return response;
	}

	const response = forwardWithAuth(request, tokens.access_token);
	response.cookies.set("access_token", tokens.access_token, {
		httpOnly: true,
		secure: process.env.NODE_ENV === "production",
		sameSite: "lax",
		maxAge: tokens.expires_in ?? 900,
		path: "/",
	});
	response.cookies.set("refresh_token", tokens.refresh_token, {
		httpOnly: true,
		secure: process.env.NODE_ENV === "production",
		sameSite: "lax",
		maxAge: tokens.refresh_expires_in ?? 36000,
		path: "/",
	});
	return response;
}

export const config = {
	matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
