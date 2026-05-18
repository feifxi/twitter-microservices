"use client";

import { format } from "date-fns";
import { Calendar, Link2, MapPin } from "lucide-react";
import Link from "next/link";
import { useSelectedLayoutSegment } from "next/navigation";
import { useState } from "react";
import { Avatar } from "@/components/Avatar";
import { EditProfileModal } from "@/components/EditProfileModal";
import type { ProfileTab } from "@/components/ProfileTabContent";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTab } from "@/components/ui/tabs";
import { useFollow, useProfile, useUnfollow } from "@/hooks/useUser";
import type { User } from "@/lib/types";
import { displayNameOf, formatCount } from "@/lib/utils";

interface ProfileHeaderProps {
	user: User;
	currentUserId: string;
}

// "posts" lives at /profile/:id (no trailing tab). Others are /profile/:id/<tab>.
function tabHref(userId: string, tab: ProfileTab): string {
	return tab === "posts" ? `/profile/${userId}` : `/profile/${userId}/${tab}`;
}

const TWEET_TABS: ReadonlySet<ProfileTab> = new Set([
	"posts",
	"replies",
	"media",
	"likes",
]);

// Derive the active tab from the URL via useSelectedLayoutSegment so the
// header stays mounted across tab switches (Next caches layouts, so re-keying
// off a prop would cause the whole component to remount).
function isProfileTab(value: string | null): value is ProfileTab {
	return value !== null && TWEET_TABS.has(value as ProfileTab);
}

export function ProfileHeader({
	user: initialUser,
	currentUserId,
}: ProfileHeaderProps) {
	const segment = useSelectedLayoutSegment();
	const activeTab: ProfileTab = isProfileTab(segment) ? segment : "posts";
	const [editOpen, setEditOpen] = useState(false);
	// Subscribe to the cache so optimistic follow/unfollow updates re-render
	// the button. Falls back to the SSR-prefetched initialUser before mount.
	const { data: live } = useProfile(initialUser.id);
	const user = live ?? initialUser;
	const followMutation = useFollow(user.id);
	const unfollowMutation = useUnfollow(user.id);

	const isOwn = user.id === currentUserId;
	const displayName = displayNameOf(user);

	function handleFollowClick() {
		if (user.is_following) {
			unfollowMutation.mutate();
		} else {
			followMutation.mutate();
		}
	}

	return (
		<>
			{/* Banner */}
			<div className="relative h-[200px] bg-muted">
				{user.header_image_url ? (
					// eslint-disable-next-line @next/next/no-img-element
					<img
						src={user.header_image_url}
						alt="Profile banner"
						className="h-full w-full object-cover"
					/>
				) : (
					<div className="h-full w-full bg-banner" />
				)}
			</div>

			{/* Avatar row — relative so it stacks above the banner (both positioned, later wins) */}
			<div className="relative flex items-start justify-between px-4 pb-3 mt-[-68px]">
				<div className="rounded-full border-4 border-background bg-background">
					<Avatar src={user.avatar_url} name={displayName} size="lg" />
				</div>

				<div className="mt-[76px]">
					{isOwn ? (
						<Button
							variant="outline"
							size="sm"
							onClick={() => setEditOpen(true)}
							className="rounded-full px-4 font-bold"
						>
							Edit profile
						</Button>
					) : (
						<Button
							variant={user.is_following ? "outline" : "default"}
							size="sm"
							onClick={handleFollowClick}
							className="rounded-full px-4 font-bold"
						>
							{user.is_following ? "Following" : "Follow"}
						</Button>
					)}
				</div>
			</div>

			{/* Bio section */}
			<div className="px-4">
				<p className="text-[20px] font-extrabold leading-tight">
					{displayName}
				</p>
				{user.username && (
					<p className="text-[15px] text-muted-foreground">@{user.username}</p>
				)}

				{user.bio && (
					<p className="mt-3 text-[15px] leading-5 whitespace-pre-wrap">
						{user.bio}
					</p>
				)}

				{/* Meta: location, website, joined */}
				<div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[15px] text-muted-foreground">
					{user.location && (
						<span className="flex items-center gap-1">
							<MapPin className="h-4 w-4 shrink-0" />
							{user.location}
						</span>
					)}
					{user.website_url && (
						<a
							href={user.website_url}
							target="_blank"
							rel="noopener noreferrer"
							onClick={(e) => e.stopPropagation()}
							className="flex items-center gap-1 text-primary hover:underline"
						>
							<Link2 className="h-4 w-4 shrink-0" />
							{user.website_url.replace(/^https?:\/\//, "")}
						</a>
					)}
					<span className="flex items-center gap-1">
						<Calendar className="h-4 w-4 shrink-0" />
						Joined {format(new Date(user.created_at), "MMMM yyyy")}
					</span>
				</div>

				{/* Follow counts */}
				<div className="mt-3 flex gap-5 text-[15px]">
					<Link
						href={`/profile/${user.id}/following`}
						className="hover:underline"
					>
						<strong>{formatCount(user.following_count)}</strong>{" "}
						<span className="text-muted-foreground">Following</span>
					</Link>
					<Link
						href={`/profile/${user.id}/followers`}
						className="hover:underline"
					>
						<strong>{formatCount(user.follower_count)}</strong>{" "}
						<span className="text-muted-foreground">
							{user.follower_count === 1 ? "Follower" : "Followers"}
						</span>
					</Link>
				</div>
			</div>

			{/* Tabs are URL-driven: each TabsTab renders as a <Link> (anchor).
			    nativeButton={false} tells Base UI we're rendering a non-<button>
			    element; otherwise it warns about lost button semantics.
			    "Likes" is owner-only — Twitter parity. */}
			<Tabs value={activeTab} className="mt-4">
				<TabsList>
					<TabsTab
						value="posts"
						nativeButton={false}
						render={<Link href={tabHref(user.id, "posts")} />}
					>
						Posts
					</TabsTab>
					<TabsTab
						value="replies"
						nativeButton={false}
						render={<Link href={tabHref(user.id, "replies")} />}
					>
						Replies
					</TabsTab>
					<TabsTab
						value="media"
						nativeButton={false}
						render={<Link href={tabHref(user.id, "media")} />}
					>
						Media
					</TabsTab>
					{isOwn && (
						<TabsTab
							value="likes"
							nativeButton={false}
							render={<Link href={tabHref(user.id, "likes")} />}
						>
							Likes
						</TabsTab>
					)}
				</TabsList>
			</Tabs>

			{editOpen && (
				<EditProfileModal user={user} onClose={() => setEditOpen(false)} />
			)}
		</>
	);
}
