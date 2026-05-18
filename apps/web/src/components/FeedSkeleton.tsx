import { TweetSkeleton } from "@/components/TweetSkeleton";

export function FeedSkeleton() {
	return (
		<div>
			{/* Tab bar */}
			<div className="sticky top-0 z-10 flex border-b border-border bg-background/80 backdrop-blur-sm">
				<div className="flex-1 py-4 flex justify-center">
					<div className="h-4 w-16 rounded bg-muted animate-pulse" />
				</div>
				<div className="flex-1 py-4 flex justify-center">
					<div className="h-4 w-20 rounded bg-muted animate-pulse" />
				</div>
			</div>
			{/* Composer skeleton */}
			<div className="flex gap-3 border-b border-border px-4 py-3 animate-pulse">
				<div className="h-10 w-10 shrink-0 rounded-full bg-muted" />
				<div className="flex-1 pt-2">
					<div className="h-5 w-48 rounded bg-muted" />
					<div className="mt-4 h-8 w-16 rounded-full bg-muted self-end ml-auto" />
				</div>
			</div>
			{/* Tweet rows */}
			{Array.from({ length: 6 }).map((_, i) => (
				<TweetSkeleton key={i} />
			))}
		</div>
	);
}
