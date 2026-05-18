"use client";

import Link from "next/link";
import { Avatar } from "@/components/Avatar";
import { Button } from "@/components/ui/button";
import { useFollow, useUnfollow } from "@/hooks/useUser";
import type { UserListItem } from "@/lib/types";
import { displayNameOf } from "@/lib/utils";

export function UserRowSkeleton() {
	return (
		<div className="flex animate-pulse items-start gap-3 border-b border-border px-4 py-3">
			<div className="h-10 w-10 shrink-0 rounded-full bg-muted" />
			<div className="flex-1 space-y-2 pt-1">
				<div className="h-3 w-28 rounded bg-muted" />
				<div className="h-3 w-20 rounded bg-muted" />
				<div className="h-3 w-3/4 rounded bg-muted" />
			</div>
		</div>
	);
}

interface UserRowProps {
	user: UserListItem;
	isSelf?: boolean;
	// "default" — full row with bio (followers/following/suggestions pages)
	// "compact" — small avatar, no bio (sidebar "Who to follow")
	variant?: "default" | "compact";
}

export function UserRow({ user, isSelf, variant = "default" }: UserRowProps) {
	const follow = useFollow(user.id);
	const unfollow = useUnfollow(user.id);
	const displayName = displayNameOf(user);

	function handleFollow(e: React.MouseEvent) {
		e.preventDefault();
		if (user.is_following) unfollow.mutate();
		else follow.mutate();
	}

	if (variant === "compact") {
		return (
			<Link
				href={`/profile/${user.id}`}
				className="flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-muted/60"
			>
				<div className="flex min-w-0 items-center gap-3">
					<Avatar src={user.avatar_url} name={displayName} size="sm" />
					<div className="min-w-0">
						<p className="truncate text-[15px] font-bold leading-tight">
							{displayName}
						</p>
						{user.username && (
							<p className="truncate text-[13px] text-muted-foreground">
								@{user.username}
							</p>
						)}
					</div>
				</div>
				{!isSelf && (
					<Button
						size="sm"
						variant={user.is_following ? "outline" : "default"}
						onClick={handleFollow}
						className="shrink-0 rounded-full px-4 font-bold"
					>
						{user.is_following ? "Following" : "Follow"}
					</Button>
				)}
			</Link>
		);
	}

	return (
		<Link
			href={`/profile/${user.id}`}
			className="flex items-start gap-3 border-b border-border px-4 py-3 transition-colors hover:bg-muted/30"
		>
			<Avatar src={user.avatar_url} name={displayName} />
			<div className="min-w-0 flex-1">
				<div className="flex items-start justify-between gap-2">
					<div className="min-w-0">
						<p className="truncate text-[15px] font-bold leading-tight">
							{displayName}
						</p>
						{user.username && (
							<p className="truncate text-[15px] text-muted-foreground">
								@{user.username}
							</p>
						)}
					</div>
					{!isSelf && (
						<Button
							size="sm"
							variant={user.is_following ? "outline" : "default"}
							onClick={handleFollow}
							className="shrink-0 rounded-full px-4 font-bold"
						>
							{user.is_following ? "Following" : "Follow"}
						</Button>
					)}
				</div>
				{user.bio && (
					<p className="mt-1 line-clamp-2 text-[15px]">{user.bio}</p>
				)}
			</div>
		</Link>
	);
}
