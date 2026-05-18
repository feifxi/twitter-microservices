export function jwtSub(token: string): string | null {
	try {
		const payload = token.split(".")[1];
		const json = Buffer.from(payload, "base64url").toString("utf-8");
		const sub = JSON.parse(json).sub;
		return typeof sub === "string" ? sub : null;
	} catch {
		return null;
	}
}

export function jwtExp(token: string): number | null {
	try {
		const payload = token.split(".")[1];
		const json = Buffer.from(payload, "base64url").toString("utf-8");
		const exp = JSON.parse(json).exp;
		return typeof exp === "number" ? exp : null;
	} catch {
		return null;
	}
}
