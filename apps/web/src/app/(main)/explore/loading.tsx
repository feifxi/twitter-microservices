import { TweetSkeleton } from "@/components/TweetSkeleton";

export default function ExploreLoading() {
	return (
		<div>
			<div className="sticky top-0 z-10 border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div className="h-9 rounded-full bg-muted animate-pulse" />
			</div>
			<div className="flex border-b border-border">
				<div className="flex-1 py-3 flex justify-center">
					<div className="h-4 w-14 rounded bg-muted animate-pulse" />
				</div>
				<div className="flex-1 py-3 flex justify-center">
					<div className="h-4 w-14 rounded bg-muted animate-pulse" />
				</div>
			</div>
			{Array.from({ length: 6 }).map((_, i) => (
				<TweetSkeleton key={i} />
			))}
		</div>
	);
}
