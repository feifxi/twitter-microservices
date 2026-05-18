import axios, { type AxiosError, type InternalAxiosRequestConfig } from "axios";

const api = axios.create({
	baseURL: "",
	headers: { "Content-Type": "application/json" },
});

// Silent refresh on 401: call /api/auth/refresh (server-side, reads
// refresh_token httpOnly cookie), then retry the original request once.
// `_retried` prevents loops if the refresh itself returns 401.
type RetryConfig = InternalAxiosRequestConfig & { _retried?: boolean };

let refreshInFlight: Promise<boolean> | null = null;

async function refreshOnce(): Promise<boolean> {
	if (!refreshInFlight) {
		refreshInFlight = fetch("/api/auth/refresh", {
			method: "POST",
			credentials: "include",
		})
			.then((r) => r.ok)
			.catch(() => false)
			.finally(() => {
				refreshInFlight = null;
			});
	}
	return refreshInFlight;
}

api.interceptors.response.use(undefined, async (error: AxiosError) => {
	const original = error.config as RetryConfig | undefined;
	if (!original || error.response?.status !== 401 || original._retried) {
		throw error;
	}
	original._retried = true;
	const ok = await refreshOnce();
	if (!ok) {
		// Schedule the redirect so the throw propagates first; otherwise the
		// navigation aborts in-flight React error handlers mid-update.
		queueMicrotask(() => {
			window.location.href = "/login";
		});
		throw error;
	}
	return api(original);
});

export default api;
