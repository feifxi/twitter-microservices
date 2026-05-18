"use client";

import { format } from "date-fns";
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
import { TweetActionsMenu } from "@/components/TweetActionsMenu";
import { TweetActionButton } from "@/components/TweetActionButton";
import { useDeleteTweet, useLike, useRetweet } from "@/hooks/useTweet";
import { renderTweetBody } from "@/lib/render-tweet-body";
import type { Tweet } from "@/lib/types";
import { cn, displayNameOf, formatCount } from "@/lib/utils";
import { toast } from "@/store/toast";

interface TweetDetailProps {
	tweet: Tweet;
	currentUserId: string;
}

export function TweetDetail({ tweet, currentUserId }: TweetDetailProps) {
	const router = useRouter();
	const likeMutation = useLike();
	const retweetMutation = useRetweet();
	const deleteMutation = useDeleteTweet();
	const [lightboxOpen, setLightboxOpen] = useState(false);
	const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

	const author = tweet.author;
	const authorName = displayNameOf(author);
	const isOwn = author.id === currentUserId;

	function handleShare() {
		const url = `${window.location.origin}/tweet/${tweet.id}`;
		navigator.clipboard
			.writeText(url)
			.then(() => toast.show("Copied to clipboard"));
	}

	function handleRepostFromMenu() {
		const wasRetweeted = tweet.is_retweeted;
		retweetMutation.mutate({ tweetId: tweet.id });
		toast.show(wasRetweeted ? "Repost removed." : "Reposted.");
	}

	function handleDeleteConfirm() {
		deleteMutation.mutate(tweet.id, {
			onSuccess: () => {
				setDeleteConfirmOpen(false);
				toast.show("Your post was deleted.");
				router.push("/home");
			},
		});
	}

	return (
		<>
			<div className="border-b border-border px-4 pt-3 pb-1">
				{/* Author row */}
				<div className="flex items-center justify-between">
					<Link
						href={`/profile/${author.id}`}
						className="group flex items-center gap-3"
					>
						<Avatar src={author.avatar_url} name={authorName} />
						<div>
							<p className="text-[15px] font-bold leading-tight group-hover:underline">
								{authorName}
							</p>
							{author.username && (
								<p className="text-[15px] text-muted-foreground">
									@{author.username}
								</p>
							)}
						</div>
					</Link>

					{/* 3-dot menu */}
					<TweetActionsMenu
						isOwn={isOwn}
						isRetweeted={tweet.is_retweeted}
						onRepost={handleRepostFromMenu}
						onDelete={() => setDeleteConfirmOpen(true)}
					/>
				</div>

				{/* Body */}
				{tweet.body && (
					<p className="mt-3 min-w-0 whitespace-pre-wrap wrap-anywhere text-[23px] leading-7">
						{renderTweetBody(tweet.body)}
					</p>
				)}

				{/* Media */}
				{tweet.media_url && (
					// eslint-disable-next-line @next/next/no-img-element
					<img
						src={tweet.media_url}
						alt="Tweet media"
						onClick={() => setLightboxOpen(true)}
						className="mt-3 max-h-[512px] w-full cursor-zoom-in rounded-2xl border border-border object-cover"
					/>
				)}

				{/* Timestamp */}
				<time className="mt-4 block text-[15px] text-muted-foreground">
					{format(new Date(tweet.created_at), "h:mm a · MMM d, yyyy")}
				</time>

				{/* Stats row */}
				{(tweet.retweet_count > 0 ||
					tweet.like_count > 0 ||
					tweet.reply_count > 0) && (
					<div className="mt-3 flex gap-5 border-t border-border pt-3 text-[15px]">
						{tweet.reply_count > 0 && (
							<span>
								<strong>{formatCount(tweet.reply_count)}</strong>{" "}
								<span className="text-muted-foreground">
									{tweet.reply_count === 1 ? "Reply" : "Replies"}
								</span>
							</span>
						)}
						{tweet.retweet_count > 0 && (
							<span>
								<strong>{formatCount(tweet.retweet_count)}</strong>{" "}
								<span className="text-muted-foreground">Reposts</span>
							</span>
						)}
						{tweet.like_count > 0 && (
							<span>
								<strong>{formatCount(tweet.like_count)}</strong>{" "}
								<span className="text-muted-foreground">
									{tweet.like_count === 1 ? "Like" : "Likes"}
								</span>
							</span>
						)}
					</div>
				)}

				{/* Action bar */}
				<div className="mt-1 flex items-center justify-around border-t border-border pt-1">
					<TweetActionButton
						icon={<MessageCircle className="h-5 w-5" />}
						label="Reply"
						size="md"
					/>
					<TweetActionButton
						icon={<Repeat2 className="h-5 w-5" />}
						label={tweet.is_retweeted ? "Undo repost" : "Repost"}
						active={tweet.is_retweeted}
						color="green"
						size="md"
						onClick={() => retweetMutation.mutate({ tweetId: tweet.id })}
					/>
					<TweetActionButton
						icon={
							<Heart
								className={cn("h-5 w-5", tweet.is_liked && "fill-current")}
							/>
						}
						label={tweet.is_liked ? "Unlike" : "Like"}
						active={tweet.is_liked}
						color="pink"
						size="md"
						onClick={() => likeMutation.mutate({ tweetId: tweet.id })}
					/>
					<TweetActionButton
						icon={<BarChart2 className="h-5 w-5" />}
						label="View analytics"
						size="md"
					/>
					<TweetActionButton
						icon={<Bookmark className="h-5 w-5" />}
						label="Bookmark"
						size="md"
					/>
					<TweetActionButton
						icon={<Share className="h-5 w-5" />}
						label="Share"
						size="md"
						onClick={handleShare}
					/>
				</div>
			</div>

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
