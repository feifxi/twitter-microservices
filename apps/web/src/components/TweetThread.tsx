"use client";

import { TweetCard } from "@/components/TweetCard";
import { TweetComposer } from "@/components/TweetComposer";
import { TweetDetail } from "@/components/TweetDetail";
import { TweetSkeleton } from "@/components/TweetSkeleton";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";
import { useReplies, useTweet } from "@/hooks/useTweet";

interface TweetThreadProps {
	tweetId: string;
	currentUserId: string;
}

export function TweetThread({ tweetId, currentUserId }: TweetThreadProps) {
	const { data: tweet, isLoading: tweetLoading } = useTweet(tweetId);
	const repliesQuery = useReplies(tweetId);
	const sentinelRef = useInfiniteScroll(repliesQuery);

	const replies = repliesQuery.data?.pages.flatMap((p) => p.items) ?? [];

	if (tweetLoading) {
		return (
			<div className="space-y-0">
				{Array.from({ length: 3 }).map((_, i) => (
					<TweetSkeleton key={i} />
				))}
			</div>
		);
	}

	if (!tweet) {
		return (
			<div className="px-4 py-8 text-center text-sm text-muted-foreground">
				Tweet not found.
			</div>
		);
	}

	return (
		<div>
			<TweetDetail tweet={tweet} currentUserId={currentUserId} />

			<TweetComposer
				currentUserId={currentUserId}
				replyToId={tweetId}
				replyToUsername={tweet.author.username ?? undefined}
			/>

			{repliesQuery.isLoading && (
				<>
					{Array.from({ length: 3 }).map((_, i) => (
						<TweetSkeleton key={i} />
					))}
				</>
			)}

			{!repliesQuery.isLoading && replies.length === 0 && (
				<div className="px-4 py-8 text-center text-sm text-muted-foreground">
					No replies yet. Be the first to reply.
				</div>
			)}

			{replies.map((item) => (
				<TweetCard
					key={item.kind === "retweet" ? item.retweet.id : item.tweet.id}
					item={item}
					currentUserId={currentUserId}
				/>
			))}

			{repliesQuery.isFetchingNextPage && <TweetSkeleton />}

			<div ref={sentinelRef} className="h-px" />
		</div>
	);
}
