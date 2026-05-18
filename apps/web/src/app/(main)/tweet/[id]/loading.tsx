import { TweetSkeleton } from "@/components/TweetSkeleton";

export default function TweetLoading() {
	return (
		<div>
			{/* Header */}
			<div className="sticky top-0 z-10 flex items-center gap-6 bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div className="h-9 w-9 rounded-full bg-muted animate-pulse" />
				<div className="h-5 w-12 rounded bg-muted animate-pulse" />
			</div>

			{/* Detail block */}
			<div className="border-b border-border px-4 pt-3 pb-3 space-y-3 animate-pulse">
				<div className="flex items-center gap-3">
					<div className="h-10 w-10 rounded-full bg-muted" />
					<div className="space-y-1">
						<div className="h-4 w-32 rounded bg-muted" />
						<div className="h-3 w-20 rounded bg-muted" />
					</div>
				</div>
				<div className="space-y-2">
					<div className="h-5 w-full rounded bg-muted" />
					<div className="h-5 w-4/5 rounded bg-muted" />
					<div className="h-5 w-3/5 rounded bg-muted" />
				</div>
			</div>

			{/* Replies */}
			{Array.from({ length: 3 }).map((_, i) => (
				<TweetSkeleton key={i} />
			))}
		</div>
	);
}
