export function TweetSkeleton() {
	return (
		<div className="flex animate-pulse gap-3 border-b border-border px-4 py-3">
			<div className="h-10 w-10 shrink-0 rounded-full bg-muted" />
			<div className="min-w-0 flex-1 space-y-2 pt-1">
				<div className="flex gap-2">
					<div className="h-3 w-28 rounded bg-muted" />
					<div className="h-3 w-20 rounded bg-muted" />
					<div className="h-3 w-10 rounded bg-muted" />
				</div>
				<div className="space-y-1.5">
					<div className="h-3 w-full rounded bg-muted" />
					<div className="h-3 w-4/5 rounded bg-muted" />
				</div>
				<div className="flex max-w-md items-center justify-between pt-1">
					<div className="h-4 w-10 rounded bg-muted" />
					<div className="h-4 w-10 rounded bg-muted" />
					<div className="h-4 w-10 rounded bg-muted" />
				</div>
			</div>
		</div>
	);
}
