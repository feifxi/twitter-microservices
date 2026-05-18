"use client";

import {
	type InfiniteData,
	useInfiniteQuery,
	useMutation,
	useQueryClient,
} from "@tanstack/react-query";
import api from "@/lib/axios";
import { keys } from "@/lib/query-keys";
import type { NotificationsResponse } from "@/lib/types";
import { useNotificationStore } from "@/store/notification";

type NotificationsCache = InfiniteData<
	NotificationsResponse,
	string | undefined
>;

async function fetchNotifications(
	cursor?: string,
): Promise<NotificationsResponse> {
	const params = new URLSearchParams({ limit: "20" });
	if (cursor) params.set("cursor", cursor);
	const res = await api.get<NotificationsResponse>(
		`/v1/notifications?${params}`,
	);
	return res.data;
}

export function useNotifications() {
	return useInfiniteQuery({
		queryKey: keys.notifications(),
		queryFn: ({ pageParam }) => fetchNotifications(pageParam),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
	});
}

// Optimistically stamp read_at on the matching notification in-place.
// Avoids refetching every page of the infinite list on a single click.
function stampRead(
	data: NotificationsCache | undefined,
	id: string,
	now: string,
): NotificationsCache | undefined {
	if (!data) return data;
	return {
		...data,
		pages: data.pages.map((page) => ({
			...page,
			notifications: page.notifications.map((n) =>
				n.id === id ? { ...n, read_at: n.read_at ?? now } : n,
			),
		})),
	};
}

function stampAllRead(
	data: NotificationsCache | undefined,
	now: string,
): NotificationsCache | undefined {
	if (!data) return data;
	return {
		...data,
		pages: data.pages.map((page) => ({
			...page,
			notifications: page.notifications.map((n) => ({
				...n,
				read_at: n.read_at ?? now,
			})),
		})),
	};
}

export function useMarkRead() {
	const queryClient = useQueryClient();
	const decrement = useNotificationStore((s) => s.decrement);
	return useMutation({
		mutationFn: (id: string) => api.patch(`/v1/notifications/${id}/read`),
		onSuccess: (_data, id) => {
			decrement();
			queryClient.setQueryData<NotificationsCache>(
				keys.notifications(),
				(old) => stampRead(old, id, new Date().toISOString()),
			);
		},
	});
}

export function useMarkAllRead() {
	const queryClient = useQueryClient();
	const clear = useNotificationStore((s) => s.clear);
	return useMutation({
		mutationFn: () => api.patch("/v1/notifications/read"),
		onSuccess: () => {
			clear();
			queryClient.setQueryData<NotificationsCache>(
				keys.notifications(),
				(old) => stampAllRead(old, new Date().toISOString()),
			);
		},
	});
}
