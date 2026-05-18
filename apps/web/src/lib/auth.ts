import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { jwtSub } from "@/lib/jwt";

export async function requireUserId(): Promise<string> {
	const cookieStore = await cookies();
	const token = cookieStore.get("access_token")?.value;
	if (!token) redirect("/login");
	const userId = jwtSub(token);
	if (!userId) redirect("/login?error=invalid_token");
	return userId;
}
