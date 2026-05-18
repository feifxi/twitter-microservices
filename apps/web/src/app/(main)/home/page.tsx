import type { InfiniteData } from "@tanstack/react-query";
import {
	dehydrate,
	HydrationBoundary,
	QueryClient,
} from "@tanstack/react-query";
import { TweetFeed } from "@/components/TweetFeed";
import { requireUserId } from "@/lib/auth";
import { normalizeTweetList, type RawTweetListResponse } from "@/lib/normalize";
import { keys } from "@/lib/query-keys";
import { serverGet } from "@/lib/server-api";
import type { FeedResponse, User } from "@/lib/types";

export default async function HomePage() {
	const userId = await requireUserId();
	const queryClient = new QueryClient();

	await Promise.allSettled([
		serverGet<RawTweetListResponse>("/v1/feed/recommended?limit=20").then(
			(raw) => {
				queryClient.setQueryData<
					InfiniteData<FeedResponse, string | undefined>
				>(keys.feed.recommended(), {
					pages: [normalizeTweetList(raw)],
					pageParams: [undefined],
				});
			},
		),
		serverGet<User>(`/v1/users/${userId}`).then((user) => {
			queryClient.setQueryData<User>(keys.users.detail(userId), user);
		}),
	]);

	return (
		<HydrationBoundary state={dehydrate(queryClient)}>
			<TweetFeed currentUserId={userId} />
		</HydrationBoundary>
	);
}
