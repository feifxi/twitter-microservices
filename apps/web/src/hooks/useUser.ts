"use client";

import {
	type InfiniteData,
	type QueryClient,
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "@tanstack/react-query";
import api from "@/lib/axios";
import { debouncedToggle } from "@/lib/debounced-toggle";
import { normalizeFeed, type RawFeedResponse } from "@/lib/normalize";
import { keys, matchers, type ProfileTweetsFilter } from "@/lib/query-keys";
import type { ProfileInput } from "@/lib/schemas";
import type {
	FeedResponse,
	SearchUser,
	SearchUsersResponse,
	User,
	UserListItem,
	UserListResponse,
} from "@/lib/types";

interface RawUser {
	id: string;
	username: string | null;
	display_name: string | null;
	bio: string | null;
	avatar_url: string | null;
	header_image_url: string | null;
	website_url: string | null;
	location: string | null;
	follower_count: number;
	following_count: number;
	is_following: boolean;
	created_at: string;
}

function normalizeUser(raw: RawUser): User {
	return {
		id: raw.id,
		username: raw.username,
		display_name: raw.display_name,
		bio: raw.bio,
		avatar_url: raw.avatar_url,
		header_image_url: raw.header_image_url,
		website_url: raw.website_url,
		location: raw.location,
		follower_count: raw.follower_count,
		following_count: raw.following_count,
		is_following: raw.is_following,
		created_at: raw.created_at,
	};
}

async function fetchProfile(id: string): Promise<User> {
	const res = await api.get<RawUser>(`/v1/users/${id}`);
	return normalizeUser(res.data);
}

export function useProfile(id: string) {
	return useQuery({
		queryKey: keys.users.detail(id),
		queryFn: () => fetchProfile(id),
		enabled: !!id,
	});
}

export function useUpdateProfile(userId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (data: ProfileInput) =>
			api
				.patch<RawUser>(`/v1/users/me`, data)
				.then((r) => normalizeUser(r.data)),
		onSuccess: (updatedUser) => {
			queryClient.setQueryData<User>(keys.users.detail(userId), (prev) => ({
				...(prev ?? updatedUser),
				...updatedUser,
				is_following: prev?.is_following ?? updatedUser.is_following,
			}));
			const listPatch: Partial<UserListItem> = {
				username: updatedUser.username,
				display_name: updatedUser.display_name,
				avatar_url: updatedUser.avatar_url,
				bio: updatedUser.bio,
			};
			queryClient.setQueriesData<
				InfiniteData<UserListResponse, string | undefined>
			>({ predicate: (q) => matchers.userConnections(q.queryKey) }, (old) =>
				applyUserListUpdate(old, userId, listPatch),
			);
			queryClient.setQueriesData<UserListItem[]>(
				{ predicate: (q) => matchers.userSuggestions(q.queryKey) },
				(old) =>
					old?.map((u) => (u.id === userId ? { ...u, ...listPatch } : u)),
			);
		},
	});
}

// useFollow and useUnfollow both delegate to followToggle, which shares one
// debounce scope per target user. That way clicking Follow→Unfollow rapidly
// (different hook instances, same conceptual toggle) collapses to one net call.
//
// is_following lives in several caches across the app — single-user, all
// user-list pages (followers/following), suggestions, and search results. We
// propagate the optimistic update to ALL of them so the button reflects state
// everywhere the user appears, not just on the profile page.

interface UserListPageShape<U> {
	users: U[];
	next_cursor: string | null;
}

function applyUserListUpdate<U extends { id: string }>(
	data: InfiniteData<UserListPageShape<U>, string | undefined> | undefined,
	targetId: string,
	patch: Partial<U>,
): InfiniteData<UserListPageShape<U>, string | undefined> | undefined {
	if (!data) return data;
	return {
		...data,
		pages: data.pages.map((page) => ({
			...page,
			users: page.users.map((u) =>
				u.id === targetId ? { ...u, ...patch } : u,
			),
		})),
	};
}

function applyFollowUpdate(
	qc: QueryClient,
	targetId: string,
	intendedState: boolean,
) {
	// Single user (profile page) — shape: User
	qc.setQueryData<User>(keys.users.detail(targetId), (old) =>
		old
			? {
					...old,
					is_following: intendedState,
					follower_count: Math.max(
						0,
						old.follower_count + (intendedState ? 1 : -1),
					),
				}
			: old,
	);
	// Followers/following pages — shape: InfiniteData<UserListResponse>
	qc.setQueriesData<InfiniteData<UserListResponse, string | undefined>>(
		{ predicate: (q) => matchers.userConnections(q.queryKey) },
		(old) =>
			applyUserListUpdate(old, targetId, { is_following: intendedState }),
	);
	// Sidebar + Follow page suggestions — shape: UserListItem[] (flat, not infinite)
	qc.setQueriesData<UserListItem[]>(
		{ predicate: (q) => matchers.userSuggestions(q.queryKey) },
		(old) =>
			old?.map((u) =>
				u.id === targetId ? { ...u, is_following: intendedState } : u,
			),
	);
	// Explore search results — shape: InfiniteData<SearchUsersResponse>
	qc.setQueriesData<InfiniteData<SearchUsersResponse, string | undefined>>(
		{ predicate: (q) => matchers.searchUsers(q.queryKey) },
		(old) =>
			applyUserListUpdate<SearchUser>(old, targetId, {
				is_following: intendedState,
			}),
	);
}

function followToggle(
	queryClient: QueryClient,
	targetId: string,
	intendedState: boolean,
) {
	const previousUser = queryClient.getQueryData<User>(
		keys.users.detail(targetId),
	);
	const currentState = previousUser?.is_following ?? !intendedState;

	applyFollowUpdate(queryClient, targetId, intendedState);

	debouncedToggle(
		`follow:${targetId}`,
		currentState,
		intendedState,
		async (finalState) => {
			try {
				if (finalState) await api.post(`/v1/users/${targetId}/follow`);
				else await api.delete(`/v1/users/${targetId}/follow`);
			} finally {
				queryClient.invalidateQueries({
					queryKey: keys.users.detail(targetId),
				});
			}
		},
	);
}

export function useFollow(targetId: string) {
	const queryClient = useQueryClient();
	return { mutate: () => followToggle(queryClient, targetId, true) };
}

export function useUnfollow(targetId: string) {
	const queryClient = useQueryClient();
	return { mutate: () => followToggle(queryClient, targetId, false) };
}

function useUserListInfinite(
	queryKey: readonly unknown[],
	path: string,
	enabled: boolean,
) {
	return useInfiniteQuery({
		queryKey,
		queryFn: async ({ pageParam }): Promise<UserListResponse> => {
			const params = new URLSearchParams({ limit: "20" });
			if (pageParam) params.set("cursor", pageParam);
			const res = await api.get<UserListResponse>(`${path}?${params}`);
			return res.data;
		},
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled,
	});
}

export function useFollowers(userId: string) {
	return useUserListInfinite(
		keys.users.followers(userId),
		`/v1/users/${userId}/followers`,
		!!userId,
	);
}

export function useFollowing(userId: string) {
	return useUserListInfinite(
		keys.users.following(userId),
		`/v1/users/${userId}/following`,
		!!userId,
	);
}

export function useSuggestions(limit = 5) {
	return useQuery({
		queryKey: keys.users.suggestions(limit),
		queryFn: async () => {
			const res = await api.get<UserListResponse>(
				`/v1/users/suggestions?limit=${limit}`,
			);
			return res.data.users;
		},
		// Suggestions reshuffle on refetch — keep stale data for 5 min so the
		// sidebar doesn't refetch on every navigation.
		staleTime: 5 * 60 * 1000,
	});
}

export function useUserTweets(
	userId: string,
	filter: ProfileTweetsFilter = "posts",
) {
	return useInfiniteQuery({
		queryKey: keys.users.tweets(userId, filter),
		queryFn: async ({ pageParam }): Promise<FeedResponse> => {
			const params = new URLSearchParams({ limit: "20", filter });
			if (pageParam) params.set("cursor", pageParam);
			const res = await api.get<RawFeedResponse>(
				`/v1/users/${userId}/tweets?${params}`,
			);
			return normalizeFeed(res.data);
		},
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled: !!userId,
	});
}

// Owner-only — backend returns 403 if viewer != userId. Caller passes
// `enabled: viewerIsOwner` to avoid the wasted request on non-owners.
export function useUserLikes(userId: string, enabled: boolean) {
	return useInfiniteQuery({
		queryKey: keys.users.likes(userId),
		queryFn: async ({ pageParam }): Promise<FeedResponse> => {
			const params = new URLSearchParams({ limit: "20" });
			if (pageParam) params.set("cursor", pageParam);
			const res = await api.get<RawFeedResponse>(
				`/v1/users/${userId}/likes?${params}`,
			);
			return normalizeFeed(res.data);
		},
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
		initialPageParam: undefined as string | undefined,
		enabled: enabled && !!userId,
	});
}
