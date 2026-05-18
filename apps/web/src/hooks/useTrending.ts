"use client";

import { useQuery } from "@tanstack/react-query";
import api from "@/lib/axios";
import { keys } from "@/lib/query-keys";
import type { TrendingResponse } from "@/lib/types";

async function fetchTrending(limit: number): Promise<TrendingResponse> {
	const res = await api.get<TrendingResponse>(
		`/v1/feed/trending?limit=${limit}`,
	);
	return res.data;
}

export function useTrending(limit = 10) {
	return useQuery({
		queryKey: keys.trending(limit),
		queryFn: () => fetchTrending(limit),
		refetchInterval: 30_000,
	});
}
