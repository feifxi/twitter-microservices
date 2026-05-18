import { cookies } from "next/headers";
import { KONG_URL } from "@/lib/config";

export async function serverGet<T>(path: string): Promise<T> {
	const cookieStore = await cookies();
	const token = cookieStore.get("access_token")?.value;

	const res = await fetch(`${KONG_URL}${path}`, {
		headers: token ? { Authorization: `Bearer ${token}` } : {},
		cache: "no-store",
	});

	if (!res.ok) throw new Error(`API ${res.status}: ${path}`);
	return res.json() as Promise<T>;
}
