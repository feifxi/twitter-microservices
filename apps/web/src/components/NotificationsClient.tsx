"use client";

import { formatDistanceToNow } from "date-fns";
import { Heart, MessageCircle, Repeat2, UserPlus } from "lucide-react";
import Link from "next/link";
import { Avatar } from "@/components/Avatar";
import { TweetSkeleton } from "@/components/TweetSkeleton";
import { useInfiniteScroll } from "@/hooks/useInfiniteScroll";
import {
	useMarkAllRead,
	useMarkRead,
	useNotifications,
} from "@/hooks/useNotifications";
import type { Notification } from "@/lib/types";
import { cn, displayNameOf } from "@/lib/utils";

const typeConfig = {
	like: {
		icon: Heart,
		bgColor: "bg-like",
		label: "liked your post",
	},
	retweet: {
		icon: Repeat2,
		bgColor: "bg-retweet",
		label: "reposted your post",
	},
	reply: {
		icon: MessageCircle,
		bgColor: "bg-primary",
		label: "replied to your post",
	},
	follow: {
		icon: UserPlus,
		bgColor: "bg-primary",
		label: "followed you",
	},
} as const;

function NotificationRow({ notification }: { notification: Notification }) {
	const config = typeConfig[notification.type];
	const Icon = config.icon;
	const markRead = useMarkRead();
	const actorName = displayNameOf(
		{
			display_name: notification.actor_display_name,
			username: notification.actor_username,
		},
		"Someone",
	);
	const href =
		notification.type === "follow"
			? `/profile/${notification.actor_id}`
			: notification.tweet_id
				? `/tweet/${notification.tweet_id}`
				: "#";

	function handleClick() {
		if (!notification.read_at) markRead.mutate(notification.id);
	}

	return (
		<Link
			href={href}
			onClick={handleClick}
			className={cn(
				"flex items-start gap-4 border-b border-border px-4 py-3 transition-colors hover:bg-muted/30",
				!notification.read_at && "bg-primary/3",
			)}
		>
			{/* Icon badge + avatar */}
			<div className="flex flex-col items-center gap-1 pt-0.5">
				<span
					className={cn(
						"flex h-8 w-8 items-center justify-center rounded-full text-white",
						config.bgColor,
					)}
				>
					<Icon className="h-[18px] w-[18px]" />
				</span>
			</div>

			<div className="min-w-0 flex-1">
				{/* Actor avatar + unread dot */}
				<div className="mb-2 flex items-center justify-between">
					<Avatar
						src={notification.actor_avatar_url}
						name={actorName}
						size="sm"
					/>
					{!notification.read_at && (
						<span className="h-2 w-2 rounded-full bg-primary" />
					)}
				</div>

				<p className="text-[15px]">
					<span className="font-bold">{actorName}</span>{" "}
					<span className="text-muted-foreground">{config.label}</span>
				</p>

				{notification.tweet_preview && (
					<p className="mt-1 truncate text-[15px] text-muted-foreground">
						{notification.tweet_preview}
					</p>
				)}

				<p className="mt-1 text-[13px] text-muted-foreground">
					{formatDistanceToNow(new Date(notification.created_at), {
						addSuffix: true,
					})}
				</p>
			</div>
		</Link>
	);
}

export function NotificationsClient() {
	const query = useNotifications();
	const markAllRead = useMarkAllRead();
	const sentinelRef = useInfiniteScroll(query);

	const notifications = query.data?.pages.flatMap((p) => p.notifications) ?? [];
	const hasUnread = notifications.some((n) => !n.read_at);

	return (
		<div>
			{/* Header */}
			<div className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<h1 className="text-[20px] font-extrabold">Notifications</h1>
				{hasUnread && (
					<button
						type="button"
						disabled={markAllRead.isPending}
						onClick={() => markAllRead.mutate()}
						className="cursor-pointer text-[15px] font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50"
					>
						Mark all as read
					</button>
				)}
			</div>

			{query.isLoading && (
				<>
					{Array.from({ length: 8 }).map((_, i) => (
						<TweetSkeleton key={i} />
					))}
				</>
			)}

			{query.isError && (
				<div className="flex flex-col items-center gap-3 px-4 py-12 text-center">
					<p className="text-[15px] text-muted-foreground">
						Failed to load notifications.
					</p>
					<button
						onClick={() => query.refetch()}
						className="cursor-pointer text-[15px] font-medium text-primary hover:underline"
					>
						Try again
					</button>
				</div>
			)}

			{!query.isLoading && !query.isError && notifications.length === 0 && (
				<div className="px-8 py-12">
					<h2 className="text-[31px] font-extrabold">
						Nothing to see here — yet
					</h2>
					<p className="mt-2 text-[15px] text-muted-foreground">
						From likes to reposts and a whole lot more, this is where all the
						action happens.
					</p>
				</div>
			)}

			{notifications.map((n) => (
				<NotificationRow key={n.id} notification={n} />
			))}

			{query.isFetchingNextPage && <TweetSkeleton />}

			<div ref={sentinelRef} className="h-px" />
		</div>
	);
}
