"use client";

import {
	type InfiniteData,
	type QueryClient,
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "@tanstack/react-query";
import api from "@/lib/axios";
import { debouncedToggle } from "@/lib/debounced-toggle";
import {
	normalizeTweet,
	normalizeTweetList,
	type RawTweet,
	type RawTweetListResponse,
} from "@/lib/normalize";
import { keys, matchers } from "@/lib/query-keys";
import type { TweetInput } from "@/lib/schemas";
import type { FeedResponse, SearchTweetsResponse, Tweet } from "@/lib/types";

// Tweets live in differently-shaped caches; one pass updates them all.
// Cache key shapes and the predicates that match them live in lib/query-keys.

function mapFeedPages(
	data: InfiniteData<FeedResponse, string | undefined> | undefined,
	mapItems: (items: FeedResponse["items"]) => FeedResponse["items"],
): InfiniteData<FeedResponse, string | undefined> | undefined {
	if (!data) return data;
	return {
		...data,
		pages: data.pages.map((page) => ({ ...page, items: mapItems(page.items) })),
	};
}

function mapSearchPages(
	data: InfiniteData<SearchTweetsResponse, string | undefined> | undefined,
	mapTweets: (tweets: Tweet[]) => Tweet[],
): InfiniteData<SearchTweetsResponse, string | undefined> | undefined {
	if (!data) return data;
	return {
		...data,
		pages: data.pages.map((page) => ({
			...page,
			tweets: mapTweets(page.tweets),
		})),
	};
}

function applyTweetUpdate(
	qc: QueryClient,
	tweetId: string,
	update: Partial<Tweet>,
) {
	qc.setQueryData<Tweet>(keys.tweets.detail(tweetId), (old) =>
		old ? { ...old, ...update } : old,
	);
	qc.setQueriesData<InfiniteData<FeedResponse, string | undefined>>(
		{ predicate: (q) => matchers.feedShaped(q.queryKey) },
		(old) =>
			mapFeedPages(old, (items) =>
				items.map((item) =>
					item.tweet.id === tweetId
						? { ...item, tweet: { ...item.tweet, ...update } }
						: item,
				),
			),
	);
	qc.setQueriesData<InfiniteData<SearchTweetsResponse, string | undefined>>(
		{ predicate: (q) => matchers.searchTweets(q.queryKey) },
		(old) =>
			mapSearchPages(old, (tweets) =>
				tweets.map((t) => (t.id === tweetId ? { ...t, ...update } : t)),
			),
	);
}

function removeTweetEverywhere(qc: QueryClient, tweetId: string) {
	qc.removeQueries({ queryKey: keys.tweets.detail(tweetId) });
	qc.setQueriesData<InfiniteData<FeedResponse, string | undefined>>(
		{ predicate: (q) => matchers.feedShaped(q.queryKey) },
		(old) =>
			mapFeedPages(old, (items) =>
				items.filter((item) => item.tweet.id !== tweetId),
			),
	);
	qc.setQueriesData<InfiniteData<SearchTweetsResponse, string | undefined>>(
		{ predicate: (q) => matchers.searchTweets(q.queryKey) },
		(old) =>
			mapSearchPages(old, (tweets) => tweets.filter((t) => t.id !== tweetId)),
	);
}

// Reads the canonical Tweet from whichever cache has it, so toggles compute
// from the latest snapshot and converge correctly across rapid clicks.
function findCachedTweet(qc: QueryClient, tweetId: string): Tweet | undefined {
	const single = qc.getQueryData<Tweet>(keys.tweets.detail(tweetId));
	if (single) return single;
	for (const [, data] of qc.getQueriesData<
		InfiniteData<FeedResponse, string | undefined>
	>({ predicate: (q) => matchers.feedShaped(q.queryKey) })) {
		for (const page of data?.pages ?? []) {
			const hit = page.items.find((it) => it.tweet.id === tweetId);
			if (hit) return hit.tweet;
		}
	}
	for (const [, data] of qc.getQueriesData<
		InfiniteData<SearchTweetsResponse, string | undefined>
	>({ predicate: (q) => matchers.searchTweets(q.queryKey) })) {
		for (const page of data?.pages ?? []) {
			const hit = page.tweets.find((t) => t.id === tweetId);
			if (hit) return hit;
		}
	}
	return undefined;
}

export function useTweet(id: string) {
	return useQuery({
		queryKey: keys.tweets.detail(id),
		queryFn: async () => {
			const res = await api.get<RawTweet>(`/v1/tweets/${id}`);
			return normalizeTweet(res.data);
		},
		enabled: !!id,
	});
}

export function useReplies(tweetId: string) {
	return useInfiniteQuery({
		queryKey: keys.tweets.replies(tweetId),
		queryFn: async ({ pageParam }) => {
			const params = new URLSearchParams({ limit: "20" });
			if (pageParam) params.set("cursor", pageParam);
			const res = await api.get<RawTweetListResponse>(
				`/v1/tweets/${tweetId}/replies?${params}`,
			);
			return normalizeTweetList(res.data);
		},
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled: !!tweetId,
	});
}

export function usePostTweet() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (data: TweetInput) =>
			api
				.post<RawTweet>("/v1/tweets", data)
				.then((r) => normalizeTweet(r.data)),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: keys.feed.all() });
		},
	});
}

export function useDeleteTweet() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (tweetId: string) => api.delete(`/v1/tweets/${tweetId}`),
		onSuccess: (_data, tweetId) => {
			removeTweetEverywhere(queryClient, tweetId);
		},
	});
}

// State is read from cache, not from the caller's prop snapshot, so rapid
// clicks toggle from the latest known state and counts converge. The HTTP
// request is debounced + state-diffed (see lib/debounced-toggle.ts).

function useToggle(
	field: "is_liked" | "is_retweeted",
	countField: "like_count" | "retweet_count",
	endpoint: (id: string) => string,
) {
	const queryClient = useQueryClient();
	return {
		mutate({ tweetId }: { tweetId: string }) {
			const current = findCachedTweet(queryClient, tweetId);
			if (!current) return;
			const currentState = current[field];
			const newState = !currentState;
			applyTweetUpdate(queryClient, tweetId, {
				[field]: newState,
				[countField]: Math.max(0, current[countField] + (newState ? 1 : -1)),
			});
			debouncedToggle(
				`${field}:${tweetId}`,
				currentState,
				newState,
				async (finalState) => {
					try {
						if (finalState) await api.post(endpoint(tweetId));
						else await api.delete(endpoint(tweetId));
					} finally {
						queryClient.invalidateQueries({
							queryKey: keys.tweets.detail(tweetId),
						});
					}
				},
			);
		},
	};
}

export function useLike() {
	return useToggle("is_liked", "like_count", (id) => `/v1/tweets/${id}/like`);
}

export function useRetweet() {
	return useToggle(
		"is_retweeted",
		"retweet_count",
		(id) => `/v1/tweets/${id}/retweet`,
	);
}

export function usePostReply() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			replyToId,
			data,
		}: {
			replyToId: string;
			data: TweetInput;
		}) =>
			api
				.post<RawTweet>(`/v1/tweets/${replyToId}/reply`, data)
				.then((r) => normalizeTweet(r.data)),
		onSuccess: (_data, { replyToId }) => {
			queryClient.invalidateQueries({
				queryKey: keys.tweets.replies(replyToId),
			});
			queryClient.invalidateQueries({
				queryKey: keys.tweets.detail(replyToId),
			});
		},
	});
}
