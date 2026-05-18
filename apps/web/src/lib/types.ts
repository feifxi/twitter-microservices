export interface User {
	id: string;
	username: string | null;
	display_name: string | null;
	bio: string | null;
	avatar_url: string | null;
	header_image_url: string | null;
	website_url: string | null;
	location: string | null;
	follower_count: number;
	following_count: number;
	is_following: boolean;
	created_at: string;
}

// Author profile fields embedded on a tweet or retweet wrapper.
export interface Author {
	id: string;
	username: string | null;
	display_name: string | null;
	avatar_url: string | null;
}

// A Tweet is ALWAYS an original (or a reply). Retweets are represented by
// FeedItem with kind === "retweet" — never by mutating Tweet fields. There is
// no retweet_of_id and no original_author on Tweet.
export type TweetType = "tweet" | "reply";

export interface Tweet {
	id: string;
	type: TweetType;
	author: Author;
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

// Thin retweet reference. The retweeted tweet rides separately as
// FeedItem.tweet — engagement endpoints always take FeedItem.tweet.id, never
// RetweetWrapper.id.
export interface RetweetWrapper {
	id: string; // rt_...
	retweeter: Author;
	created_at: string;
}

export type FeedItem =
	| { kind: "tweet"; tweet: Tweet }
	| { kind: "retweet"; tweet: Tweet; retweet: RetweetWrapper };

export interface FeedResponse {
	items: FeedItem[];
	next_cursor: string | null;
}

export interface Notification {
	id: string;
	type: "like" | "retweet" | "reply" | "follow";
	actor_id: string;
	actor_username: string | null;
	actor_display_name: string | null;
	actor_avatar_url: string | null;
	tweet_id: string | null;
	tweet_preview: string | null;
	read_at: string | null;
	created_at: string;
}

export interface TrendingTag {
	tag: string;
	score: number;
}

export interface PresignResult {
	media_id: string;
	upload_url: string;
	public_url: string;
	expires_at: string;
}

export interface SearchTweetsResponse {
	tweets: Tweet[];
	next_cursor: string | null;
}

// Mirrors UserListItem so search results render through the shared UserRow.
export interface SearchUser {
	id: string;
	username: string | null;
	display_name: string | null;
	avatar_url: string | null;
	bio: string | null;
	follower_count: number;
	is_following: boolean;
}

export interface SearchUsersResponse {
	users: SearchUser[];
	next_cursor: string | null;
}

export interface NotificationsResponse {
	notifications: Notification[];
	next_cursor: string | null;
}

export interface TrendingResponse {
	trending: TrendingTag[];
}

// Slim user shape returned by /v1/users/:id/followers, .../following, and
// /v1/users/suggestions. Smaller than User — only what UserCard needs.
export interface UserListItem {
	id: string;
	username: string | null;
	display_name: string | null;
	avatar_url: string | null;
	bio: string | null;
	follower_count: number;
	is_following: boolean;
}

export interface UserListResponse {
	users: UserListItem[];
	next_cursor: string | null;
}
