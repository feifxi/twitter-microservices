"use client";

import { Search } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { UserRow } from "@/components/UserRow";
import { useTrending } from "@/hooks/useTrending";
import { useSuggestions } from "@/hooks/useUser";
import { formatScore } from "@/lib/utils";

function TrendingSkeletonRow() {
	return (
		<div className="animate-pulse space-y-1 px-4 py-3">
			<div className="h-3 w-16 rounded bg-muted" />
			<div className="h-4 w-28 rounded bg-muted" />
			<div className="h-3 w-20 rounded bg-muted" />
		</div>
	);
}

export function TrendingSidebar() {
	const { data, isLoading, isError } = useTrending();
	const suggestions = useSuggestions(5);
	const router = useRouter();
	const [query, setQuery] = useState("");

	function handleSearch(e: React.SyntheticEvent<HTMLFormElement>): void {
		e.preventDefault();
		if (query.trim()) {
			router.push(`/explore?q=${encodeURIComponent(query.trim())}`);
		}
	}

	return (
		<div className="flex h-full flex-col gap-3 overflow-y-auto px-4 py-3 scrollbar-none">
			{/* Search bar */}
			<form onSubmit={handleSearch}>
				<label className="flex items-center gap-3 rounded-full bg-muted px-4 py-2.5 ring-primary focus-within:bg-background focus-within:ring-1 transition-all">
					<Search className="h-4 w-4 shrink-0 text-muted-foreground" />
					<input
						type="text"
						value={query}
						onChange={(e) => setQuery(e.target.value)}
						placeholder="Search"
						className="min-w-0 flex-1 bg-transparent text-[15px] placeholder:text-muted-foreground focus:outline-none"
					/>
				</label>
			</form>

			{/* Trends box */}
			<div className="overflow-hidden rounded-2xl bg-muted/40">
				<div className="px-4 pt-3 pb-1">
					<h2 className="text-xl font-extrabold">What's happening</h2>
				</div>

				{isLoading && (
					<div className="pb-2">
						{Array.from({ length: 5 }).map((_, i) => (
							<TrendingSkeletonRow key={i} />
						))}
					</div>
				)}

				{isError && (
					<p className="px-4 py-4 text-sm text-muted-foreground">
						Unable to load trends.
					</p>
				)}

				{data?.trending.map((tag, index) => (
					<Link
						key={tag.tag}
						href={`/explore?q=${encodeURIComponent(tag.tag)}`}
						className="flex flex-col px-4 py-3 transition-colors hover:bg-muted/60"
					>
						<p className="text-[13px] text-muted-foreground">
							Trending · {index + 1}
						</p>
						<p className="font-bold leading-snug">{tag.tag}</p>
						<p className="mt-0.5 text-[13px] text-muted-foreground">
							{formatScore(tag.score)}
						</p>
					</Link>
				))}

				{!isLoading && !isError && data?.trending.length === 0 && (
					<p className="px-4 py-4 text-sm text-muted-foreground">
						No trends right now.
					</p>
				)}

				<Link
					href="/explore"
					className="block rounded-b-2xl px-4 py-3 text-[15px] text-primary transition-colors hover:bg-muted/60"
				>
					Show more
				</Link>
			</div>

			{/* Who to follow */}
			<div className="overflow-hidden rounded-2xl bg-muted/40">
				<div className="px-4 pt-3 pb-1">
					<h2 className="text-xl font-extrabold">Who to Follow</h2>
				</div>

				{suggestions.isLoading && (
					<div className="pb-2">
						{Array.from({ length: 3 }).map((_, i) => (
							<TrendingSkeletonRow key={i} />
						))}
					</div>
				)}

				{suggestions.data?.map((u) => (
					<UserRow key={u.id} user={u} variant="compact" />
				))}

				{!suggestions.isLoading && suggestions.data?.length === 0 && (
					<p className="px-4 py-4 text-sm text-muted-foreground">
						No suggestions right now.
					</p>
				)}

				<Link
					href="/connect"
					className="block rounded-b-2xl px-4 py-3 text-[15px] text-primary transition-colors hover:bg-muted/60"
				>
					Show more
				</Link>
			</div>

			{/* Footer links */}
			<div className="flex flex-wrap gap-x-3 gap-y-1 px-1 text-[13px] text-muted-foreground">
				{[
					"Terms of Service",
					"Privacy Policy",
					"Cookie Policy",
					"Accessibility",
					"Ads info",
				].map((label) => (
					<span key={label} className="cursor-pointer hover:underline">
						{label}
					</span>
				))}
				<span>© {new Date().getFullYear()} X Corp.</span>
			</div>
		</div>
	);
}
