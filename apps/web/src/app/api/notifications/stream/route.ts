import { cookies } from "next/headers";
import type { NextRequest } from "next/server";
import { KONG_URL } from "@/lib/config";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(request: NextRequest) {
	const cookieStore = await cookies();
	const token = cookieStore.get("access_token")?.value;

	if (!token) {
		return new Response("Unauthorized", { status: 401 });
	}

	// Forward client disconnect to upstream so Kong/Go closes its end too.
	const upstream = new AbortController();
	request.signal.addEventListener("abort", () => upstream.abort());

	let backendRes: Response;
	try {
		backendRes = await fetch(`${KONG_URL}/v1/notifications/stream`, {
			headers: {
				Authorization: `Bearer ${token}`,
				Accept: "text/event-stream",
				"Cache-Control": "no-cache",
			},
			signal: upstream.signal,
		});
	} catch {
		return new Response("Stream unavailable", { status: 502 });
	}

	if (!backendRes.ok || !backendRes.body) {
		return new Response("Stream unavailable", { status: 502 });
	}

	return new Response(backendRes.body, {
		headers: {
			"Content-Type": "text/event-stream",
			"Cache-Control": "no-cache",
			Connection: "keep-alive",
		},
	});
}
