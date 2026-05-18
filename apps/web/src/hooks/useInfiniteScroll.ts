"use client";

import { type RefObject, useEffect, useRef } from "react";

interface InfiniteScrollQuery {
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	fetchNextPage: () => void;
}

// Attach the returned ref to a sentinel <div> below the list. When the sentinel
// scrolls into view, the next page is fetched. Pass enabled=false to pause
// (e.g. while a tab is hidden).
export function useInfiniteScroll(
	query: InfiniteScrollQuery,
	enabled = true,
): RefObject<HTMLDivElement | null> {
	const ref = useRef<HTMLDivElement>(null);
	useEffect(() => {
		if (!enabled) return;
		const el = ref.current;
		if (!el) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (
					entries[0].isIntersecting &&
					query.hasNextPage &&
					!query.isFetchingNextPage
				) {
					query.fetchNextPage();
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, [query, enabled]);
	return ref;
}
