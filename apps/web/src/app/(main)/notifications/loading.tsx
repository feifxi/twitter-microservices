import { TweetSkeleton } from "@/components/TweetSkeleton";

export default function NotificationsLoading() {
	return (
		<div>
			<div className="sticky top-0 z-10 border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div className="h-6 w-28 rounded bg-muted animate-pulse" />
			</div>
			{Array.from({ length: 6 }).map((_, i) => (
				<TweetSkeleton key={i} />
			))}
		</div>
	);
}
