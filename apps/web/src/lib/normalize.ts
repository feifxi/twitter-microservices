import type {
	Author,
	FeedItem,
	FeedResponse,
	RetweetWrapper,
	Tweet,
} from "@/lib/types";

export interface RawAuthor {
	id: string;
	username: string;
	display_name: string | null;
	avatar_url: string | null;
}

// Retweets are conveyed at the feed-item layer. `type` is derived from `reply_to_id`
// in normalizeTweet — not a wire field.
export interface RawTweet {
	id: string;
	author: RawAuthor | null;
	body: string;
	reply_to_id: string | null;
	media_id: string | null;
	media_url: string | null;
	like_count: number;
	retweet_count: number;
	reply_count: number;
	hashtags: string[];
	is_liked: boolean;
	is_retweeted: boolean;
	created_at: string;
}

export interface RawRetweetWrapper {
	id: string;
	retweeter: RawAuthor;
	created_at: string;
}

export type RawFeedItem =
	| { kind: "tweet"; tweet: RawTweet }
	| { kind: "retweet"; tweet: RawTweet; retweet: RawRetweetWrapper };

export interface RawFeedResponse {
	items: RawFeedItem[];
	next_cursor: string | null;
}

// Legacy list-shape — `/v1/tweets/{id}/replies` still returns flat tweets.
export interface RawTweetListResponse {
	tweets: RawTweet[];
	next_cursor: string | null;
}

function normalizeAuthor(raw: RawAuthor | null): Author {
	if (!raw) {
		return { id: "", username: null, display_name: null, avatar_url: null };
	}
	return {
		id: raw.id,
		username: raw.username,
		display_name: raw.display_name,
		avatar_url: raw.avatar_url,
	};
}

export function normalizeTweet(raw: RawTweet): Tweet {
	const type: Tweet["type"] = raw.reply_to_id != null ? "reply" : "tweet";
	return {
		id: raw.id,
		type,
		author: normalizeAuthor(raw.author),
		body: raw.body,
		reply_to_id: raw.reply_to_id,
		media_id: raw.media_id,
		media_url: raw.media_url,
		like_count: raw.like_count,
		retweet_count: raw.retweet_count,
		reply_count: raw.reply_count,
		hashtags: raw.hashtags ?? [],
		is_liked: raw.is_liked,
		is_retweeted: raw.is_retweeted,
		created_at: raw.created_at,
	};
}

function normalizeRetweet(raw: RawRetweetWrapper): RetweetWrapper {
	return {
		id: raw.id,
		retweeter: normalizeAuthor(raw.retweeter),
		created_at: raw.created_at,
	};
}

export function normalizeFeedItem(raw: RawFeedItem): FeedItem {
	if (raw.kind === "retweet") {
		return {
			kind: "retweet",
			tweet: normalizeTweet(raw.tweet),
			retweet: normalizeRetweet(raw.retweet),
		};
	}
	return { kind: "tweet", tweet: normalizeTweet(raw.tweet) };
}

export function normalizeFeed(raw: RawFeedResponse): FeedResponse {
	return {
		items: raw.items.map(normalizeFeedItem),
		next_cursor: raw.next_cursor,
	};
}

// Helper for the flat /replies endpoint — wraps each reply as a kind:"tweet"
// feed item so the call site can render with the same components.
export function normalizeTweetList(raw: RawTweetListResponse): FeedResponse {
	return {
		items: raw.tweets.map((t) => ({ kind: "tweet", tweet: normalizeTweet(t) })),
		next_cursor: raw.next_cursor,
	};
}
