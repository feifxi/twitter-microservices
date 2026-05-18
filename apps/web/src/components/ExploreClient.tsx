"use client";

import { Search, TrendingUp } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { TweetCard } from "@/components/TweetCard";
import { TweetSkeleton } from "@/components/TweetSkeleton";
import { Tabs, TabsList, TabsTab } from "@/components/ui/tabs";
import { UserRow, UserRowSkeleton } from "@/components/UserRow";
import { useDebouncedValue } from "@/hooks/useDebouncedValue";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";
import { useSearchTweets, useSearchUsers } from "@/hooks/useSearch";
import { useTrending } from "@/hooks/useTrending";
import { formatScore } from "@/lib/utils";

function TrendingSkeleton() {
	return (
		<div className="animate-pulse space-y-1 border-b border-border px-4 py-3">
			<div className="h-3 w-20 rounded bg-muted" />
			<div className="h-4 w-32 rounded bg-muted" />
			<div className="h-3 w-16 rounded bg-muted" />
		</div>
	);
}

const TABS = ["tweets", "people"] as const;
type Tab = (typeof TABS)[number];

function isTab(value: unknown): value is Tab {
	return (
		typeof value === "string" && (TABS as readonly string[]).includes(value)
	);
}

interface ExploreClientProps {
	currentUserId: string;
	initialQ: string;
}

export function ExploreClient({ currentUserId, initialQ }: ExploreClientProps) {
	const router = useRouter();
	const searchParams = useSearchParams();
	const [inputValue, setInputValue] = useState(initialQ);
	const debouncedQ = useDebouncedValue(inputValue, 300);
	const [tab, setTab] = useState<Tab>("tweets");

	const tweetsQuery = useSearchTweets(debouncedQ);
	const usersQuery = useSearchUsers(debouncedQ);
	const trendingQuery = useTrending(20);

	const tweetSentinelRef = useInfiniteScroll(tweetsQuery, tab === "tweets");
	const peopleSentinelRef = useInfiniteScroll(usersQuery, tab === "people");

	// Sync URL → input when navigating here from a link (e.g. trending hashtag click)
	useEffect(() => {
		const urlQ = searchParams.get("q") ?? "";
		if (urlQ !== inputValue) setInputValue(urlQ);
		// Only react to external URL changes, not our own inputValue-driven updates
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [searchParams]);

	// Sync debounced search term → URL (so refresh/share preserves the query)
	useEffect(() => {
		const params = new URLSearchParams(searchParams.toString());
		if (debouncedQ) params.set("q", debouncedQ);
		else params.delete("q");
		const next = params.toString();
		if (next !== searchParams.toString()) {
			router.replace(`/explore?${next}`, { scroll: false });
		}
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [debouncedQ]);

	const tweets = tweetsQuery.data?.pages.flatMap((p) => p.tweets) ?? [];
	const users = usersQuery.data?.pages.flatMap((p) => p.users) ?? [];

	return (
		<>
			{/* Search bar */}
			<div className="sticky top-0 z-10 border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div className="flex items-center gap-3 rounded-full border border-border bg-muted/40 px-4 py-2 focus-within:border-primary focus-within:bg-background transition-colors">
					<Search className="h-4 w-4 shrink-0 text-muted-foreground" />
					<input
						autoFocus
						type="search"
						value={inputValue}
						onChange={(e) => setInputValue(e.target.value)}
						placeholder="Search"
						className="flex-1 bg-transparent text-sm placeholder:text-muted-foreground focus:outline-none"
					/>
				</div>
			</div>

			{/* Tabs — only shown when actively searching */}
			{debouncedQ && (
				<Tabs value={tab} onValueChange={(v) => isTab(v) && setTab(v)}>
					<TabsList>
						<TabsTab value="tweets">Tweets</TabsTab>
						<TabsTab value="people">People</TabsTab>
					</TabsList>
				</Tabs>
			)}

			{/* Trending — shown when no query */}
			{!debouncedQ && (
				<div>
					<div className="flex items-center gap-2 border-b border-border px-4 py-4">
						<TrendingUp className="h-5 w-5 text-primary" />
						<h2 className="text-xl font-extrabold">Trending</h2>
					</div>

					{trendingQuery.isLoading &&
						Array.from({ length: 8 }).map((_, i) => (
							<TrendingSkeleton key={i} />
						))}

					{trendingQuery.data?.trending.map((tag, index) => (
						<Link
							key={tag.tag}
							href={`/explore?q=${encodeURIComponent(tag.tag)}`}
							className="flex items-start justify-between border-b border-border px-4 py-3 transition-colors hover:bg-muted/40"
						>
							<div className="min-w-0">
								<p className="text-[13px] text-muted-foreground">
									Trending · {index + 1}
								</p>
								<p className="mt-0.5 text-[17px] font-bold leading-snug">
									{tag.tag}
								</p>
								<p className="mt-0.5 text-[13px] text-muted-foreground">
									{formatScore(tag.score)}
								</p>
							</div>
						</Link>
					))}

					{!trendingQuery.isLoading &&
						trendingQuery.data?.trending.length === 0 && (
							<p className="px-4 py-12 text-center text-sm text-muted-foreground">
								No trending topics right now.
							</p>
						)}
				</div>
			)}

			{/* Tweets tab */}
			{debouncedQ && tab === "tweets" && (
				<>
					{tweetsQuery.isLoading &&
						Array.from({ length: 5 }).map((_, i) => <TweetSkeleton key={i} />)}

					{tweetsQuery.isError && (
						<div className="flex flex-col items-center gap-3 px-4 py-12 text-center">
							<p className="text-sm text-muted-foreground">Search failed.</p>
							<button
								onClick={() => tweetsQuery.refetch()}
								className="cursor-pointer text-sm font-medium text-primary hover:underline"
							>
								Try again
							</button>
						</div>
					)}

					{!tweetsQuery.isLoading &&
						!tweetsQuery.isError &&
						tweets.length === 0 && (
							<div className="px-4 py-12 text-center text-sm text-muted-foreground">
								No tweets found for &ldquo;{debouncedQ}&rdquo;.
							</div>
						)}

					{tweets.map((tweet) => (
						<TweetCard
							key={tweet.id}
							item={{ kind: "tweet", tweet }}
							currentUserId={currentUserId}
						/>
					))}

					{tweetsQuery.isFetchingNextPage && <TweetSkeleton />}
					<div ref={tweetSentinelRef} className="h-px" />
				</>
			)}

			{/* People tab */}
			{debouncedQ && tab === "people" && (
				<>
					{usersQuery.isLoading &&
						Array.from({ length: 5 }).map((_, i) => (
							<UserRowSkeleton key={i} />
						))}

					{usersQuery.isError && (
						<div className="flex flex-col items-center gap-3 px-4 py-12 text-center">
							<p className="text-sm text-muted-foreground">Search failed.</p>
							<button
								onClick={() => usersQuery.refetch()}
								className="cursor-pointer text-sm font-medium text-primary hover:underline"
							>
								Try again
							</button>
						</div>
					)}

					{!usersQuery.isLoading &&
						!usersQuery.isError &&
						users.length === 0 && (
							<div className="px-4 py-12 text-center text-sm text-muted-foreground">
								No people found for &ldquo;{debouncedQ}&rdquo;.
							</div>
						)}

					{users.map((user) => (
						<UserRow
							key={user.id}
							user={user}
							isSelf={user.id === currentUserId}
						/>
					))}

					{usersQuery.isFetchingNextPage && <UserRowSkeleton />}
					<div ref={peopleSentinelRef} className="h-px" />
				</>
			)}
		</>
	);
}
