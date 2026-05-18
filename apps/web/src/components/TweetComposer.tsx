"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { Theme } from "emoji-picker-react";
import { ImageIcon, Smile, X } from "lucide-react";
import dynamic from "next/dynamic";
import { useTheme } from "next-themes";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { Avatar } from "@/components/Avatar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover";
import { usePostReply, usePostTweet } from "@/hooks/useTweet";
import { isFileTooLarge, useUploadMedia } from "@/hooks/useUploadMedia";
import { useProfile } from "@/hooks/useUser";
import { type TweetInput, tweetSchema } from "@/lib/schemas";
import { cn, displayNameOf } from "@/lib/utils";
import { toast } from "@/store/toast";

const EmojiPicker = dynamic(() => import("emoji-picker-react"), { ssr: false });

const MAX_CHARS = 280;
const WARN_AT = 240;

function CharCount({ remaining, max }: { remaining: number; max: number }) {
	const used = max - remaining;
	const pct = Math.min(used / max, 1);
	const size = remaining <= 20 ? 24 : 20;
	const radius = (size - 4) / 2;
	const circ = 2 * Math.PI * radius;

	const ringColor =
		remaining < 0
			? "stroke-destructive"
			: remaining <= 20
				? "stroke-yellow-400"
				: "stroke-primary";

	return (
		<span
			className="relative flex items-center justify-center"
			style={{ width: size, height: size }}
		>
			<svg width={size} height={size} className="-rotate-90">
				<circle
					cx={size / 2}
					cy={size / 2}
					r={radius}
					fill="none"
					strokeWidth={2.5}
					className="stroke-muted"
				/>
				<circle
					cx={size / 2}
					cy={size / 2}
					r={radius}
					fill="none"
					strokeWidth={2.5}
					strokeDasharray={circ}
					strokeDashoffset={circ - circ * pct}
					strokeLinecap="round"
					className={ringColor}
					style={{ transition: "stroke-dashoffset 0.1s" }}
				/>
			</svg>
			{remaining <= 20 && (
				<span
					className={cn(
						"absolute text-[11px] font-medium tabular-nums leading-none",
						remaining < 0 ? "text-destructive" : "text-muted-foreground",
					)}
				>
					{remaining}
				</span>
			)}
		</span>
	);
}

interface TweetComposerProps {
	currentUserId: string;
	replyToId?: string;
	replyToUsername?: string;
	onSuccess?: () => void;
	autoFocus?: boolean;
}

export function TweetComposer({
	currentUserId,
	replyToId,
	replyToUsername,
	onSuccess,
	autoFocus,
}: TweetComposerProps) {
	const fileRef = useRef<HTMLInputElement>(null);
	const textareaRef = useRef<HTMLTextAreaElement | null>(null);
	const [pendingFile, setPendingFile] = useState<File | null>(null);
	const [mediaPreview, setMediaPreview] = useState<string | null>(null);
	const [focused, setFocused] = useState(false);
	const { resolvedTheme } = useTheme();
	const postTweet = usePostTweet();
	const postReply = usePostReply();
	const upload = useUploadMedia();
	const { data: me } = useProfile(currentUserId);
	const isReplyMode = !!replyToId;

	const { register, handleSubmit, watch, setValue, getValues, reset } =
		useForm<TweetInput>({
			resolver: zodResolver(tweetSchema),
			defaultValues: { body: "" },
		});

	const { ref: formRef, ...registerRest } = register("body");

	const body = watch("body") ?? "";
	const remaining = MAX_CHARS - body.length;
	const isOverLimit = remaining < 0;
	const showCount = body.length > 0;
	const showBar = body.length >= WARN_AT || isOverLimit;

	function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
		const file = e.target.files?.[0];
		if (!file) return;
		if (isFileTooLarge(file)) {
			toast.error("File too large. Max 10 MB.");
			e.target.value = "";
			return;
		}
		setMediaPreview((prev) => {
			if (prev) URL.revokeObjectURL(prev);
			return URL.createObjectURL(file);
		});
		setPendingFile(file);
	}

	function clearMedia() {
		setMediaPreview((prev) => {
			if (prev) URL.revokeObjectURL(prev);
			return null;
		});
		setPendingFile(null);
		setValue("media_id", undefined);
		if (fileRef.current) fileRef.current.value = "";
	}

	const mediaPreviewRef = useRef(mediaPreview);
	mediaPreviewRef.current = mediaPreview;
	useEffect(() => {
		return () => {
			if (mediaPreviewRef.current) URL.revokeObjectURL(mediaPreviewRef.current);
		};
	}, []);

	function insertEmoji(native: string) {
		const el = textareaRef.current;
		const current = getValues("body") ?? "";
		if (el) {
			const start = el.selectionStart ?? current.length;
			const end = el.selectionEnd ?? current.length;
			const next = current.slice(0, start) + native + current.slice(end);
			setValue("body", next, { shouldValidate: true });
			requestAnimationFrame(() => {
				el.focus();
				const pos = start + native.length;
				el.setSelectionRange(pos, pos);
			});
		} else {
			setValue("body", current + native, { shouldValidate: true });
		}
	}

	async function onSubmit(data: TweetInput) {
		try {
			if (pendingFile) {
				const result = await upload.mutateAsync(pendingFile);
				data.media_id = result.media_id;
				data.media_url = result.public_url;
			}

			if (isReplyMode && replyToId) {
				await postReply.mutateAsync({ replyToId, data });
				toast.show("Your reply was sent.");
			} else {
				await postTweet.mutateAsync(data);
				toast.show("Your post was sent.");
			}
			reset();
			clearMedia();
			setFocused(false);
			onSuccess?.();
		} catch {
			toast.error("Couldn't post. Please try again.");
		}
	}

	const isPending =
		postTweet.isPending || postReply.isPending || upload.isPending;
	const isDisabled = (!body.trim() && !pendingFile) || isOverLimit || isPending;
	const displayName = displayNameOf(me, "?");

	return (
		<form
			onSubmit={handleSubmit(onSubmit)}
			className="border-b border-border px-4 pb-2 pt-3"
		>
			{isReplyMode && replyToUsername && (
				<p className="mb-2 ml-[52px] text-[13px] text-muted-foreground">
					Replying to <span className="text-primary">@{replyToUsername}</span>
				</p>
			)}

			<div className="flex gap-3">
				<div className="shrink-0 pt-0.5">
					<Avatar src={me?.avatar_url ?? null} name={displayName} />
				</div>

				<div className="min-w-0 flex-1">
					<textarea
						{...registerRest}
						ref={(el) => {
							formRef(el);
							textareaRef.current = el;
						}}
						placeholder={
							isReplyMode ? "Post your reply" : "What is happening?!"
						}
						rows={isReplyMode || focused ? 3 : 2}
						onFocus={() => setFocused(true)}
						autoFocus={autoFocus}
						className="w-full resize-none bg-transparent text-[20px] leading-6 placeholder:text-muted-foreground focus:outline-none"
					/>

					{mediaPreview && (
						<div className="relative mt-2 inline-block w-full">
							{/* eslint-disable-next-line @next/next/no-img-element */}
							<img
								src={mediaPreview}
								alt="Upload preview"
								className="max-h-72 w-full rounded-2xl object-cover"
							/>
							<button
								type="button"
								onClick={clearMedia}
								className="absolute right-2 top-2 flex h-7 w-7 cursor-pointer items-center justify-center rounded-full bg-black/70 text-white backdrop-blur-sm hover:bg-black/90"
							>
								<X className="h-4 w-4" />
							</button>
						</div>
					)}

					{(focused || isReplyMode) && (
						<div className="mt-2 border-t border-border" />
					)}

					<div className="mt-2 flex items-center justify-between">
						<div className="flex items-center gap-0.5">
							<input
								ref={fileRef}
								type="file"
								accept="image/jpeg,image/png,image/gif,image/webp,video/mp4"
								className="hidden"
								onChange={handleFileChange}
							/>
							<button
								type="button"
								onClick={() => fileRef.current?.click()}
								disabled={isPending}
								className="cursor-pointer rounded-full p-2 text-primary transition-colors hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-50"
								aria-label="Add media"
							>
								<ImageIcon className="h-5 w-5" />
							</button>

							<Popover>
								<PopoverTrigger
									render={
										<button
											type="button"
											aria-label="Add emoji"
											className="cursor-pointer rounded-full p-2 text-primary transition-colors hover:bg-primary/10"
										>
											<Smile className="h-5 w-5" />
										</button>
									}
								/>
								<PopoverContent>
									<EmojiPicker
										onEmojiClick={(data) => insertEmoji(data.emoji)}
										height={450}
										width={350}
										theme={resolvedTheme === "dark" ? Theme.DARK : Theme.LIGHT}
										previewConfig={{ showPreview: false }}
										searchPlaceholder="Search emoji…"
									/>
								</PopoverContent>
							</Popover>
						</div>

						<div className="flex items-center gap-3">
							{showCount && <CharCount remaining={remaining} max={MAX_CHARS} />}
							{showBar && <span className="h-6 w-px bg-border" />}
							<button
								type="submit"
								disabled={isDisabled}
								className="cursor-pointer rounded-full bg-primary px-4 py-1.5 text-[15px] font-bold text-primary-foreground transition-all duration-150 hover:opacity-90 active:scale-95 disabled:cursor-not-allowed disabled:opacity-50"
							>
								{isPending ? "Posting…" : isReplyMode ? "Reply" : "Post"}
							</button>
						</div>
					</div>
				</div>
			</div>
		</form>
	);
}
