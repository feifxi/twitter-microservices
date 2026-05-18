"use client";

import { type InfiniteData, useInfiniteQuery } from "@tanstack/react-query";
import api from "@/lib/axios";
import { normalizeFeed, type RawFeedResponse } from "@/lib/normalize";
import { keys } from "@/lib/query-keys";
import type { FeedResponse } from "@/lib/types";

async function fetchFeed(
	endpoint: "following" | "recommended",
	cursor?: string,
): Promise<FeedResponse> {
	const params = new URLSearchParams({ limit: "20" });
	if (cursor) params.set("cursor", cursor);
	const res = await api.get<RawFeedResponse>(`/v1/feed/${endpoint}?${params}`);
	return normalizeFeed(res.data);
}

export type FeedInfiniteData = InfiniteData<FeedResponse, string | undefined>;

export function useFollowingFeed() {
	return useInfiniteQuery({
		queryKey: keys.feed.following(),
		queryFn: ({ pageParam }) => fetchFeed("following", pageParam),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
	});
}

export function useRecommendedFeed() {
	return useInfiniteQuery({
		queryKey: keys.feed.recommended(),
		queryFn: ({ pageParam }) => fetchFeed("recommended", pageParam),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
	});
}
