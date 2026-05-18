"use client";

import { useEffect } from "react";
import { z } from "zod";
import { useNotificationStore } from "@/store/notification";

// Caps exponential backoff at 30s. EventSource reconnects automatically on
// transient network errors but NOT on 5xx — we handle that here.
const MAX_BACKOFF_MS = 30_000;

const countEventSchema = z.object({ count: z.number() });

export function useNotificationSSE() {
	const increment = useNotificationStore((s) => s.increment);
	const setCount = useNotificationStore((s) => s.setCount);

	useEffect(() => {
		let es: EventSource | null = null;
		let retryMs = 1_000;
		let retryTimer: ReturnType<typeof setTimeout> | null = null;
		let closed = false;

		function connect() {
			if (closed) return;
			es = new EventSource("/api/notifications/stream");

			es.addEventListener("notification", () => increment());
			es.addEventListener("count", (e: MessageEvent) => {
				try {
					const parsed = countEventSchema.safeParse(JSON.parse(e.data));
					if (parsed.success) setCount(parsed.data.count);
				} catch {
					// malformed event — ignore
				}
			});
			es.addEventListener("open", () => {
				retryMs = 1_000;
			});
			es.addEventListener("error", () => {
				es?.close();
				es = null;
				retryTimer = setTimeout(connect, retryMs);
				retryMs = Math.min(retryMs * 2, MAX_BACKOFF_MS);
			});
		}

		connect();
		return () => {
			closed = true;
			if (retryTimer) clearTimeout(retryTimer);
			es?.close();
		};
	}, [increment, setCount]);
}
