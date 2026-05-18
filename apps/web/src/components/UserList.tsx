"use client";

import type {
	InfiniteData,
	UseInfiniteQueryResult,
} from "@tanstack/react-query";
import { UserRow, UserRowSkeleton } from "@/components/UserRow";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";
import type { UserListResponse } from "@/lib/types";

interface UserListProps {
	query: UseInfiniteQueryResult<InfiniteData<UserListResponse>, Error>;
	currentUserId: string;
	emptyMessage: string;
}

export function UserList({
	query,
	currentUserId,
	emptyMessage,
}: UserListProps) {
	const sentinelRef = useInfiniteScroll(query);
	const users = query.data?.pages.flatMap((p) => p.users) ?? [];

	if (query.isLoading) {
		return (
			<>
				{Array.from({ length: 6 }).map((_, i) => (
					<UserRowSkeleton key={i} />
				))}
			</>
		);
	}

	if (query.isError) {
		return (
			<div className="px-4 py-12 text-center">
				<p className="text-[15px] text-muted-foreground">Failed to load.</p>
				<button
					type="button"
					onClick={() => query.refetch()}
					className="mt-2 cursor-pointer text-[15px] font-medium text-primary hover:underline"
				>
					Try again
				</button>
			</div>
		);
	}

	if (users.length === 0) {
		return (
			<div className="px-8 py-12">
				<p className="text-[15px] text-muted-foreground">{emptyMessage}</p>
			</div>
		);
	}

	return (
		<>
			{users.map((u) => (
				<UserRow key={u.id} user={u} isSelf={u.id === currentUserId} />
			))}
			{query.isFetchingNextPage && <UserRowSkeleton />}
			<div ref={sentinelRef} className="h-px" />
		</>
	);
}
