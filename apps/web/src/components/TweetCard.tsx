"use client";

import {
	BarChart2,
	Bookmark,
	Heart,
	MessageCircle,
	Repeat2,
	Share,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Avatar } from "@/components/Avatar";
import { ConfirmModal } from "@/components/ConfirmModal";
import { ImageLightbox } from "@/components/ImageLightbox";
import { TweetActionButton } from "@/components/TweetActionButton";
import { TweetActionsMenu } from "@/components/TweetActionsMenu";
import { useDeleteTweet, useLike, useRetweet } from "@/hooks/useTweet";
import { renderTweetBody } from "@/lib/render-tweet-body";
import type { FeedItem } from "@/lib/types";
import { cn, displayNameOf, formatTime } from "@/lib/utils";
import { toast } from "@/store/toast";

interface TweetCardProps {
	item: FeedItem;
	currentUserId: string;
}

export function TweetCard({ item, currentUserId }: TweetCardProps) {
	const router = useRouter();
	const likeMutation = useLike();
	const retweetMutation = useRetweet();
	const deleteMutation = useDeleteTweet();
	const [lightboxOpen, setLightboxOpen] = useState(false);
	const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

	const tweet = item.tweet;
	const author = tweet.author;
	const authorName = displayNameOf(author);
	const isOwn = author.id === currentUserId;
	const reposter = item.kind === "retweet" ? item.retweet.retweeter : null;
	const reposterName = displayNameOf(reposter);

	function handleCardClick() {
		router.push(`/tweet/${tweet.id}`);
	}

	function handleLike(e: React.MouseEvent) {
		e.stopPropagation();
		likeMutation.mutate({ tweetId: tweet.id });
	}

	function handleRetweet(e: React.MouseEvent) {
		e.stopPropagation();
		retweetMutation.mutate({ tweetId: tweet.id });
	}

	function handleRepostFromMenu() {
		const wasRetweeted = tweet.is_retweeted;
		retweetMutation.mutate({ tweetId: tweet.id });
		toast.show(wasRetweeted ? "Repost removed." : "Reposted.");
	}

	function handleShare(e: React.MouseEvent) {
		e.stopPropagation();
		const url = `${window.location.origin}/tweet/${tweet.id}`;
		navigator.clipboard.writeText(url).then(() => {
			toast.show("Copied to clipboard");
		});
	}

	function handleDeleteConfirm() {
		deleteMutation.mutate(tweet.id, {
			onSuccess: () => {
				setDeleteConfirmOpen(false);
				toast.show("Your post was deleted.");
			},
		});
	}

	return (
		<>
			<article
				onClick={handleCardClick}
				className="relative flex cursor-pointer flex-col gap-0 border-b border-border px-4 py-3 transition-colors hover:bg-muted/30"
			>
				{/* Repost header */}
				{reposter && (
					<div className="mb-1 ml-10 flex items-center gap-1.5 text-[13px] font-bold text-muted-foreground">
						<Repeat2 className="h-4 w-4 shrink-0" />
						<span className="truncate">{reposterName} reposted</span>
					</div>
				)}

				<div className="flex gap-3">
					{/* Avatar */}
					<Link
						href={`/profile/${author.id}`}
						onClick={(e) => e.stopPropagation()}
						className="shrink-0 self-start"
					>
						<Avatar src={author.avatar_url} name={authorName} />
					</Link>

					{/* Content */}
					<div className="min-w-0 flex-1">
						{/* Header */}
						<div className="relative flex min-w-0 items-center justify-between gap-1">
							<div className="flex min-w-0 items-baseline gap-1 text-[15px]">
								<Link
									href={`/profile/${author.id}`}
									onClick={(e) => e.stopPropagation()}
									className="truncate font-bold hover:underline"
								>
									{authorName}
								</Link>
								{author.username && (
									<span className="min-w-0 shrink truncate text-muted-foreground">
										@{author.username}
									</span>
								)}
								<span className="shrink-0 text-muted-foreground">·</span>
								<span className="shrink-0 text-muted-foreground">
									{formatTime(tweet.created_at)}
								</span>
							</div>

							{/* 3-dot menu */}
							<div className="shrink-0">
								<TweetActionsMenu
									isOwn={isOwn}
									isRetweeted={tweet.is_retweeted}
									onRepost={handleRepostFromMenu}
									onDelete={() => setDeleteConfirmOpen(true)}
								/>
							</div>
						</div>

						{/* Body */}
						{tweet.body && (
							<p className="mt-0.5 min-w-0 whitespace-pre-wrap wrap-anywhere text-[15px] leading-5">
								{renderTweetBody(tweet.body, true)}
							</p>
						)}

						{/* Media */}
						{tweet.media_url && (
							// eslint-disable-next-line @next/next/no-img-element
							<img
								src={tweet.media_url}
								alt="Tweet media"
								onClick={(e) => {
									e.stopPropagation();
									setLightboxOpen(true);
								}}
								className="mt-3 max-h-[512px] w-full cursor-zoom-in rounded-2xl border border-border object-cover"
							/>
						)}

						{/* Action bar */}
						<div className="mt-3 flex items-center justify-between text-muted-foreground">
							<TweetActionButton
								icon={<MessageCircle className="h-[18px] w-[18px]" />}
								count={tweet.reply_count}
								label="Reply"
								asLink={`/tweet/${tweet.id}`}
							/>
							<TweetActionButton
								icon={<Repeat2 className="h-[18px] w-[18px]" />}
								count={tweet.retweet_count}
								label={tweet.is_retweeted ? "Undo repost" : "Repost"}
								active={tweet.is_retweeted}
								color="green"
								onClick={handleRetweet}
							/>
							<TweetActionButton
								icon={
									<Heart
										className={cn(
											"h-[18px] w-[18px]",
											tweet.is_liked && "fill-current",
										)}
									/>
								}
								count={tweet.like_count}
								label={tweet.is_liked ? "Unlike" : "Like"}
								active={tweet.is_liked}
								color="pink"
								onClick={handleLike}
							/>
							<TweetActionButton
								icon={<BarChart2 className="h-[18px] w-[18px]" />}
								label="View analytics"
								onClick={(e) => e.stopPropagation()}
							/>
							<div className="flex items-center gap-0.5">
								<TweetActionButton
									icon={<Bookmark className="h-[18px] w-[18px]" />}
									label="Bookmark"
									onClick={(e) => e.stopPropagation()}
								/>
								<TweetActionButton
									icon={<Share className="h-[18px] w-[18px]" />}
									label="Share"
									onClick={handleShare}
								/>
							</div>
						</div>
					</div>
				</div>
			</article>

			{lightboxOpen && tweet.media_url && (
				<ImageLightbox
					src={tweet.media_url}
					alt="Tweet media"
					onClose={() => setLightboxOpen(false)}
				/>
			)}

			{deleteConfirmOpen && (
				<ConfirmModal
					title="Delete post?"
					description="This can't be undone and it will be removed from your profile, the timeline of any accounts that follow you, and from search results."
					confirmLabel="Delete"
					danger
					loading={deleteMutation.isPending}
					onConfirm={handleDeleteConfirm}
					onClose={() => setDeleteConfirmOpen(false)}
				/>
			)}
		</>
	);
}
