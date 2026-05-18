import { TweetSkeleton } from "@/components/TweetSkeleton";

export default function ProfileLoading() {
	return (
		<div>
			{/* Header: back + name skeleton */}
			<div className="sticky top-0 z-10 flex items-center gap-6 bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div className="h-9 w-9 rounded-full bg-muted animate-pulse" />
				<div className="h-5 w-32 rounded bg-muted animate-pulse" />
			</div>

			{/* Banner */}
			<div className="h-[200px] w-full bg-banner animate-pulse" />

			{/* Avatar row */}
			<div className="relative flex items-start justify-between px-4 pb-3 mt-[-68px]">
				<div className="h-32 w-32 rounded-full border-4 border-background bg-muted animate-pulse" />
				<div className="mt-[76px] h-8 w-24 rounded-full bg-muted animate-pulse" />
			</div>

			{/* Bio */}
			<div className="space-y-2 px-4">
				<div className="h-6 w-40 rounded bg-muted animate-pulse" />
				<div className="h-4 w-28 rounded bg-muted animate-pulse" />
				<div className="h-4 w-full rounded bg-muted animate-pulse" />
			</div>

			{/* Tab strip placeholder */}
			<div className="mt-4 border-b border-border px-4 py-3">
				<div className="h-4 w-12 rounded bg-muted animate-pulse" />
			</div>

			{/* Tweet rows */}
			{Array.from({ length: 4 }).map((_, i) => (
				<TweetSkeleton key={i} />
			))}
		</div>
	);
}
