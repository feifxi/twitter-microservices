import type { InfiniteData } from "@tanstack/react-query";
import {
	dehydrate,
	HydrationBoundary,
	QueryClient,
} from "@tanstack/react-query";
import { BackButton } from "@/components/BackButton";
import { TweetThread } from "@/components/TweetThread";
import { requireUserId } from "@/lib/auth";
import {
	normalizeTweet,
	normalizeTweetList,
	type RawTweet,
	type RawTweetListResponse,
} from "@/lib/normalize";
import { keys } from "@/lib/query-keys";
import { serverGet } from "@/lib/server-api";
import type { FeedResponse } from "@/lib/types";

export default async function TweetPage({
	params,
}: {
	params: Promise<{ id: string }>;
}) {
	const { id } = await params;
	const userId = await requireUserId();
	const queryClient = new QueryClient();

	await Promise.allSettled([
		serverGet<RawTweet>(`/v1/tweets/${id}`).then((raw) =>
			queryClient.setQueryData(keys.tweets.detail(id), normalizeTweet(raw)),
		),
		serverGet<RawTweetListResponse>(`/v1/tweets/${id}/replies?limit=20`).then(
			(raw) =>
				queryClient.setQueryData<
					InfiniteData<FeedResponse, string | undefined>
				>(keys.tweets.replies(id), {
					pages: [normalizeTweetList(raw)],
					pageParams: [undefined],
				}),
		),
	]);

	return (
		<div>
			<header className="sticky top-0 z-10 flex items-center gap-6 bg-background/80 px-4 py-3 backdrop-blur-sm">
				<BackButton />
				<h1 className="text-xl font-bold">Post</h1>
			</header>

			<HydrationBoundary state={dehydrate(queryClient)}>
				<TweetThread tweetId={id} currentUserId={userId} />
			</HydrationBoundary>
		</div>
	);
}
