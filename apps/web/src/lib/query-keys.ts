// Single source of truth for TanStack Query keys. Inline arrays scattered
// across hooks are typo-prone: `["feeds"]` silently no-ops invalidation
// against `["feed"]`. The factory + matchers below keep keys and predicates
// in sync.
//
// Convention: each `*.all` returns the prefix. `invalidateQueries({ queryKey:
// keys.tweets.all() })` invalidates every tweet-related cache.

// Profile tab filters served by GET /v1/users/:id/tweets?filter=...
// "likes" is NOT in this union — it's a separate endpoint with its own cursor.
export type ProfileTweetsFilter = "posts" | "replies" | "media";

export const keys = {
	feed: {
		all: () => ["feed"] as const,
		following: () => ["feed", "following"] as const,
		recommended: () => ["feed", "recommended"] as const,
	},
	tweets: {
		all: () => ["tweet"] as const,
		detail: (id: string) => ["tweet", id] as const,
		replies: (id: string) => ["tweet", id, "replies"] as const,
	},
	users: {
		all: () => ["user"] as const,
		detail: (id: string) => ["user", id] as const,
		// Profile timeline tabs — "filter" is one of posts|replies|media.
		// Likes uses a separate key because cursor semantics differ (sorts by like time).
		tweets: (id: string, filter: ProfileTweetsFilter = "posts") =>
			["user", id, "tweets", filter] as const,
		likes: (id: string) => ["user", id, "likes"] as const,
		followers: (id: string) => ["user", id, "followers"] as const,
		following: (id: string) => ["user", id, "following"] as const,
		suggestions: (limit: number) => ["user-suggestions", limit] as const,
	},
	search: {
		tweets: (query: string, mode: string) =>
			["search", "tweets", query, mode] as const,
		users: (query: string) => ["search", "users", query] as const,
	},
	notifications: () => ["notifications"] as const,
	trending: (limit: number) => ["trending", limit] as const,
};

// Predicates for cross-cache propagation in optimistic updates. Kept here so
// they stay aligned with the key shapes above — if a key changes, update the
// matcher in the same place.
export const matchers = {
	// Caches whose pages have `.items[].tweet` (FeedResponse shape).
	feedShaped: (key: readonly unknown[]): boolean =>
		key[0] === "feed" ||
		(key[0] === "user" && (key[2] === "tweets" || key[2] === "likes")) ||
		(key[0] === "tweet" && key[2] === "replies"),

	// Caches whose pages have `.tweets[]` (SearchTweetsResponse shape).
	searchTweets: (key: readonly unknown[]): boolean =>
		key[0] === "search" && key[1] === "tweets",

	// Followers/following infinite lists — pages have `.users[]`.
	userConnections: (key: readonly unknown[]): boolean =>
		key[0] === "user" && (key[2] === "followers" || key[2] === "following"),

	// Flat suggestions array.
	userSuggestions: (key: readonly unknown[]): boolean =>
		key[0] === "user-suggestions",

	// Search users infinite list — pages have `.users[]` (SearchUsersResponse).
	searchUsers: (key: readonly unknown[]): boolean =>
		key[0] === "search" && key[1] === "users",
};
