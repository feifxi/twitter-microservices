"use client";

import Link from "next/link";
import { TweetCard } from "@/components/TweetCard";
import { TweetSkeleton } from "@/components/TweetSkeleton";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";
import { useUserLikes, useUserTweets } from "@/hooks/useUser";
import type { FeedItem } from "@/lib/types";

export type ProfileTab = "posts" | "replies" | "media" | "likes";

interface ProfileTabContentProps {
	userId: string;
	currentUserId: string;
	tab: ProfileTab;
}

const EMPTY_COPY: Record<ProfileTab, string> = {
	posts: "No tweets yet.",
	replies: "No replies yet.",
	media: "No media yet.",
	likes: "No liked tweets yet.",
};

function tweetOf(item: FeedItem) {
	return item.tweet;
}

export function ProfileTabContent({
	userId,
	currentUserId,
	tab,
}: ProfileTabContentProps) {
	const isOwner = userId === currentUserId;
	// Calling both hooks unconditionally satisfies the rules of hooks; each is
	// gated by `enabled` so only one fires per tab.
	const tweetsQuery = useUserTweets(userId, tab === "likes" ? "posts" : tab);
	const likesQuery = useUserLikes(userId, tab === "likes" && isOwner);
	const activeQuery = tab === "likes" ? likesQuery : tweetsQuery;
	const sentinelRef = useInfiniteScroll(activeQuery);
	const items = activeQuery.data?.pages.flatMap((p) => p.items) ?? [];

	if (activeQuery.isLoading) {
		if (tab === "media") {
			return (
				<div className="grid grid-cols-3 gap-0.5 p-0.5">
					{Array.from({ length: 9 }).map((_, i) => (
						<div key={i} className="aspect-square animate-pulse bg-muted" />
					))}
				</div>
			);
		}
		return (
			<>
				{Array.from({ length: 5 }).map((_, i) => (
					<TweetSkeleton key={i} />
				))}
			</>
		);
	}

	if (activeQuery.isError) {
		return (
			<div className="flex flex-col items-center gap-3 px-4 py-12 text-center">
				<p className="text-sm text-muted-foreground">Failed to load.</p>
				<button
					type="button"
					onClick={() => activeQuery.refetch()}
					className="cursor-pointer text-sm font-medium text-primary hover:underline"
				>
					Try again
				</button>
			</div>
		);
	}

	if (items.length === 0) {
		return (
			<div className="px-4 py-12 text-center text-sm text-muted-foreground">
				{EMPTY_COPY[tab]}
			</div>
		);
	}

	if (tab === "media") {
		// Media tab is Twitter-style: grid of thumbnails, click → tweet detail.
		// Items without media_url are dropped (defensive — backend filters too).
		const mediaItems = items.map(tweetOf).filter((t) => t.media_url !== null);
		return (
			<>
				<div className="grid grid-cols-3 gap-0.5 p-0.5">
					{mediaItems.map((t) => (
						<Link
							key={t.id}
							href={`/tweet/${t.id}`}
							className="relative aspect-square overflow-hidden bg-muted transition-opacity hover:opacity-90"
						>
							{/** biome-ignore lint/performance/noImgElement: external CDN, no next/image config */}
							<img
								src={t.media_url ?? ""}
								alt=""
								loading="lazy"
								className="absolute inset-0 h-full w-full object-cover"
							/>
						</Link>
					))}
				</div>
				{activeQuery.isFetchingNextPage && (
					<div className="grid grid-cols-3 gap-0.5 p-0.5">
						{Array.from({ length: 3 }).map((_, i) => (
							<div key={i} className="aspect-square animate-pulse bg-muted" />
						))}
					</div>
				)}
				<div ref={sentinelRef} className="h-px" />
			</>
		);
	}

	return (
		<>
			{items.map((item) => (
				<TweetCard
					key={item.kind === "retweet" ? item.retweet.id : item.tweet.id}
					item={item}
					currentUserId={currentUserId}
				/>
			))}

			{activeQuery.isFetchingNextPage && <TweetSkeleton />}

			<div ref={sentinelRef} className="h-px" />
		</>
	);
}
