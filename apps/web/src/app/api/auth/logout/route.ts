import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { APP_URL, KEYCLOAK_REALM, KEYCLOAK_URL } from "@/lib/config";

export async function GET() {
	const cookieStore = await cookies();
	const idToken = cookieStore.get("id_token")?.value;

	cookieStore.delete("access_token");
	cookieStore.delete("refresh_token");
	cookieStore.delete("id_token");

	if (idToken) {
		const logoutUrl = new URL(
			`${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/logout`,
		);
		logoutUrl.searchParams.set("id_token_hint", idToken);
		logoutUrl.searchParams.set("post_logout_redirect_uri", `${APP_URL}/login`);
		redirect(logoutUrl.toString());
	}

	redirect("/login");
}
