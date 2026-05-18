"use client";

import { useState } from "react";
import { TweetCard } from "@/components/TweetCard";
import { TweetComposer } from "@/components/TweetComposer";
import { TweetSkeleton } from "@/components/TweetSkeleton";
import { Tabs, TabsList, TabsTab } from "@/components/ui/tabs";
import { useFollowingFeed, useRecommendedFeed } from "@/hooks/useFeed";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";

type FeedTab = "following" | "recommended";

interface TweetFeedProps {
	currentUserId: string;
}

export function TweetFeed({ currentUserId }: TweetFeedProps) {
	const [tab, setTab] = useState<FeedTab>("recommended");

	const followingQuery = useFollowingFeed();
	const recommendedQuery = useRecommendedFeed();

	const activeQuery = tab === "following" ? followingQuery : recommendedQuery;
	const items = activeQuery.data?.pages.flatMap((p) => p.items) ?? [];
	const sentinelRef = useInfiniteScroll(activeQuery);

	return (
		<div>
			<Tabs
				value={tab}
				onValueChange={(v) => typeof v === "string" && setTab(v as FeedTab)}
				className="sticky top-0 z-10 bg-background/80 backdrop-blur-sm"
			>
				<TabsList>
					<TabsTab value="recommended">For You</TabsTab>
					<TabsTab value="following">Following</TabsTab>
				</TabsList>
			</Tabs>

			<TweetComposer currentUserId={currentUserId} />

			{activeQuery.isLoading && (
				<>
					{Array.from({ length: 5 }).map((_, i) => (
						<TweetSkeleton key={i} />
					))}
				</>
			)}

			{activeQuery.isError && (
				<div className="px-4 py-8 text-center text-sm text-muted-foreground">
					Failed to load feed.{" "}
					<button
						onClick={() => activeQuery.refetch()}
						className="cursor-pointer text-primary hover:underline"
					>
						Retry
					</button>
				</div>
			)}

			{!activeQuery.isLoading && items.length === 0 && (
				<div className="px-4 py-8 text-center text-sm text-muted-foreground">
					{tab === "following"
						? "No tweets yet. Follow some people to see their tweets here."
						: "Nothing to recommend yet. Like a few tweets to personalize your feed."}
				</div>
			)}

			{items.map((item) => (
				<TweetCard
					key={item.kind === "retweet" ? item.retweet.id : item.tweet.id}
					item={item}
					currentUserId={currentUserId}
				/>
			))}

			{activeQuery.isFetchingNextPage && <TweetSkeleton />}

			<div ref={sentinelRef} className="h-px" />
		</div>
	);
}
