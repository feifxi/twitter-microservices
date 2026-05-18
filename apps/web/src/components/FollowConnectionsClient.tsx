"use client";

import { useRouter } from "next/navigation";
import { BackButton } from "@/components/BackButton";
import { UserList } from "@/components/UserList";
import { Tabs, TabsList, TabsTab } from "@/components/ui/tabs";
import { useFollowers, useFollowing, useProfile } from "@/hooks/useUser";
import { displayNameOf } from "@/lib/utils";

type Tab = "followers" | "following";

interface FollowConnectionsClientProps {
	profileId: string;
	currentUserId: string;
	initialTab: Tab;
}

export function FollowConnectionsClient({
	profileId,
	currentUserId,
	initialTab,
}: FollowConnectionsClientProps) {
	const router = useRouter();
	// User is prefetched by the [id]/layout.tsx and handed down via HydrationBoundary.
	const { data: user } = useProfile(profileId);
	const displayName = displayNameOf(user, "Profile");
	const username = user?.username ?? null;
	const followersQuery = useFollowers(profileId);
	const followingQuery = useFollowing(profileId);

	function handleTabChange(value: unknown) {
		if (value !== "followers" && value !== "following") return;
		router.replace(`/profile/${profileId}/${value}`);
	}

	return (
		<div>
			<header className="sticky top-0 z-10 flex items-center gap-6 bg-background/80 px-4 py-3 backdrop-blur-sm">
				<BackButton />
				<div>
					<h1 className="text-xl font-bold leading-tight">{displayName}</h1>
					{username && (
						<p className="text-[13px] text-muted-foreground">@{username}</p>
					)}
				</div>
			</header>

			<Tabs value={initialTab} onValueChange={handleTabChange}>
				<TabsList>
					<TabsTab value="followers">Followers</TabsTab>
					<TabsTab value="following">Following</TabsTab>
				</TabsList>
			</Tabs>

			{initialTab === "followers" ? (
				<UserList
					query={followersQuery}
					currentUserId={currentUserId}
					emptyMessage="No followers yet."
				/>
			) : (
				<UserList
					query={followingQuery}
					currentUserId={currentUserId}
					emptyMessage="Not following anyone yet."
				/>
			)}
		</div>
	);
}
