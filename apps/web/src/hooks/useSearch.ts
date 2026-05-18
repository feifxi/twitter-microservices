"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import api from "@/lib/axios";
import { keys } from "@/lib/query-keys";
import type {
	SearchTweetsResponse,
	SearchUser,
	SearchUsersResponse,
	Tweet,
} from "@/lib/types";

// Wire shapes from search-service. Every field is always present; nullable
// fields serialize as JSON null rather than being omitted.

interface RawSearchTweet {
	id: string;
	author: {
		id: string;
		username: string | null;
		display_name: string | null;
		avatar_url: string | null;
	} | null;
	body: string;
	hashtags: string[];
	reply_to_id: string | null;
	media_id: string | null;
	media_url: string | null;
	like_count: number;
	retweet_count: number;
	reply_count: number;
	is_liked: boolean;
	is_retweeted: boolean;
	created_at: string;
}

interface RawSearchTweetsResponse {
	tweets: RawSearchTweet[];
	next_cursor: string | null;
}

interface RawSearchUser {
	id: string;
	username: string | null;
	display_name: string | null;
	avatar_url: string | null;
	bio: string | null;
	follower_count: number;
	is_following: boolean;
}

interface RawSearchUsersResponse {
	users: RawSearchUser[];
	next_cursor: string | null;
}

function normalizeSearchTweet(raw: RawSearchTweet): Tweet {
	return {
		id: raw.id,
		type: raw.reply_to_id !== null ? "reply" : "tweet",
		// Defensive: backend always sends a non-null author, but the Go field
		// is *authorSnap so the type system allows null. Empty-author fallback
		// keeps render code from crashing on a hypothetical regression.
		author: raw.author ?? {
			id: "",
			username: null,
			display_name: null,
			avatar_url: null,
		},
		body: raw.body,
		reply_to_id: raw.reply_to_id,
		media_id: raw.media_id,
		media_url: raw.media_url,
		like_count: raw.like_count,
		retweet_count: raw.retweet_count,
		reply_count: raw.reply_count,
		hashtags: raw.hashtags,
		is_liked: raw.is_liked,
		is_retweeted: raw.is_retweeted,
		created_at: raw.created_at,
	};
}

function normalizeSearchUser(raw: RawSearchUser): SearchUser {
	return {
		id: raw.id,
		username: raw.username,
		display_name: raw.display_name,
		avatar_url: raw.avatar_url,
		bio: raw.bio,
		follower_count: raw.follower_count,
		is_following: raw.is_following,
	};
}

async function fetchSearchTweets(
	query: string,
	mode: string,
	cursor?: string,
): Promise<SearchTweetsResponse> {
	const params = new URLSearchParams({ q: query, mode, limit: "20" });
	if (cursor) params.set("cursor", cursor);
	const res = await api.get<RawSearchTweetsResponse>(
		`/v1/search/tweets?${params}`,
	);
	return {
		tweets: res.data.tweets.map(normalizeSearchTweet),
		next_cursor: res.data.next_cursor,
	};
}

async function fetchSearchUsers(
	query: string,
	cursor?: string,
): Promise<SearchUsersResponse> {
	const params = new URLSearchParams({ q: query, limit: "20" });
	if (cursor) params.set("cursor", cursor);
	const res = await api.get<RawSearchUsersResponse>(
		`/v1/search/users?${params}`,
	);
	return {
		users: res.data.users.map(normalizeSearchUser),
		next_cursor: res.data.next_cursor,
	};
}

export function useSearchTweets(query: string, mode = "hybrid") {
	return useInfiniteQuery({
		queryKey: keys.search.tweets(query, mode),
		queryFn: ({ pageParam }) => fetchSearchTweets(query, mode, pageParam),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled: query.length > 0,
	});
}

export function useSearchUsers(query: string) {
	return useInfiniteQuery({
		queryKey: keys.search.users(query),
		queryFn: ({ pageParam }) => fetchSearchUsers(query, pageParam),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled: query.length > 0,
	});
}
