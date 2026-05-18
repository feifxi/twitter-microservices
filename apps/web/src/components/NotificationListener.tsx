"use client";

import { useNotificationSSE } from "@/hooks/useNotificationSSE";

export function NotificationListener() {
	useNotificationSSE();
	return null;
}
