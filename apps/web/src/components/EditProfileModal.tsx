"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { Camera, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { UseFormRegisterReturn } from "react-hook-form";
import { useForm } from "react-hook-form";
import { Avatar } from "@/components/Avatar";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogTitle,
} from "@/components/ui/dialog";
import { isFileTooLarge, useUploadMedia } from "@/hooks/useUploadMedia";
import { useUpdateProfile } from "@/hooks/useUser";
import { type ProfileInput, profileSchema } from "@/lib/schemas";
import type { User } from "@/lib/types";
import { cn, displayNameOf } from "@/lib/utils";
import { toast } from "@/store/toast";

interface EditProfileModalProps {
	user: User;
	onClose: () => void;
}

// Input must come BEFORE label in the DOM so Tailwind peer-focus (~) selector works.
interface FloatedFieldProps {
	id: string;
	label: string;
	error?: string;
	registerReturn: UseFormRegisterReturn;
	multiline?: boolean;
	maxLength?: number;
	currentLength?: number;
	type?: string;
}

function FloatedField({
	id,
	label,
	error,
	registerReturn,
	multiline,
	maxLength,
	currentLength,
	type = "text",
}: FloatedFieldProps) {
	const inputClass =
		"peer w-full bg-transparent px-3 pt-6 text-[15px] placeholder-transparent focus:outline-none";
	return (
		<div className="space-y-1">
			<div
				className={cn(
					"relative rounded-md border bg-transparent transition-colors focus-within:border-primary",
					error ? "border-destructive" : "border-border",
				)}
			>
				{multiline ? (
					<textarea
						id={id}
						rows={3}
						placeholder=" "
						maxLength={maxLength}
						{...registerReturn}
						className={cn(inputClass, "resize-none pb-2")}
					/>
				) : (
					<input
						id={id}
						type={type}
						placeholder=" "
						maxLength={maxLength}
						{...registerReturn}
						className={cn(inputClass, "pb-1.5", maxLength && "pr-16")}
					/>
				)}
				<label
					htmlFor={id}
					className={cn(
						"pointer-events-none absolute left-3 top-4 origin-left text-[15px] text-muted-foreground transition-all",
						"peer-focus:-translate-y-2.5 peer-focus:scale-75 peer-focus:text-primary",
						"peer-[:not(:placeholder-shown)]:-translate-y-2.5 peer-[:not(:placeholder-shown)]:scale-75",
					)}
				>
					{label}
				</label>
				{maxLength !== undefined && (
					// Twitter parity: counter only appears while the field is focused.
					<span
						className={cn(
							"pointer-events-none absolute right-3 text-xs text-muted-foreground opacity-0 transition-opacity peer-focus:opacity-100",
							multiline ? "bottom-2" : "top-1/2 -translate-y-1/2",
						)}
					>
						{currentLength ?? 0} / {maxLength}
					</span>
				)}
			</div>
			{error && <p className="px-1 text-xs text-destructive">{error}</p>}
		</div>
	);
}

export function EditProfileModal({ user, onClose }: EditProfileModalProps) {
	const updateProfile = useUpdateProfile(user.id);
	const upload = useUploadMedia();

	const [pendingAvatar, setPendingAvatar] = useState<File | null>(null);
	const [pendingBanner, setPendingBanner] = useState<File | null>(null);
	const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
	const [bannerPreview, setBannerPreview] = useState<string | null>(null);

	const avatarInputRef = useRef<HTMLInputElement>(null);
	const bannerInputRef = useRef<HTMLInputElement>(null);

	const {
		register,
		handleSubmit,
		watch,
		formState: { errors, isDirty },
		reset,
	} = useForm<ProfileInput>({
		resolver: zodResolver(profileSchema),
		defaultValues: {
			display_name: user.display_name ?? "",
			bio: user.bio ?? "",
			location: user.location ?? "",
			website_url: user.website_url ?? "",
		},
	});

	// Only reset if a different user opens the modal — never on cache refetch
	// of the same user, which would wipe in-progress edits.
	// biome-ignore lint/correctness/useExhaustiveDependencies: intentional — keying on user.id only
	useEffect(() => {
		reset({
			display_name: user.display_name ?? "",
			bio: user.bio ?? "",
			location: user.location ?? "",
			website_url: user.website_url ?? "",
		});
	}, [user.id, reset]);

	function handleAvatarChange(e: React.ChangeEvent<HTMLInputElement>) {
		const file = e.target.files?.[0];
		if (!file) return;
		if (isFileTooLarge(file)) {
			toast.error("File too large. Max 10 MB.");
			e.target.value = "";
			return;
		}
		setAvatarPreview((prev) => {
			if (prev) URL.revokeObjectURL(prev);
			return URL.createObjectURL(file);
		});
		setPendingAvatar(file);
	}

	function handleBannerChange(e: React.ChangeEvent<HTMLInputElement>) {
		const file = e.target.files?.[0];
		if (!file) return;
		if (isFileTooLarge(file)) {
			toast.error("File too large. Max 10 MB.");
			e.target.value = "";
			return;
		}
		setBannerPreview((prev) => {
			if (prev) URL.revokeObjectURL(prev);
			return URL.createObjectURL(file);
		});
		setPendingBanner(file);
	}

	// Refs mirror state so unmount cleanup sees the latest values without
	// re-running on every preview change.
	const avatarPreviewRef = useRef(avatarPreview);
	const bannerPreviewRef = useRef(bannerPreview);
	avatarPreviewRef.current = avatarPreview;
	bannerPreviewRef.current = bannerPreview;
	useEffect(() => {
		return () => {
			if (avatarPreviewRef.current)
				URL.revokeObjectURL(avatarPreviewRef.current);
			if (bannerPreviewRef.current)
				URL.revokeObjectURL(bannerPreviewRef.current);
		};
	}, []);

	async function onSubmit(data: ProfileInput) {
		try {
			const [avatarResult, bannerResult] = await Promise.all([
				pendingAvatar
					? upload.mutateAsync(pendingAvatar)
					: Promise.resolve(null),
				pendingBanner
					? upload.mutateAsync(pendingBanner)
					: Promise.resolve(null),
			]);
			if (avatarResult) data.avatar_url = avatarResult.public_url;
			if (bannerResult) data.header_image_url = bannerResult.public_url;
			await updateProfile.mutateAsync(data);
			onClose();
		} catch {
			toast.error("Couldn't save profile. Please try again.");
		}
	}

	const displayNameValue = watch("display_name") ?? "";
	const bioValue = watch("bio") ?? "";
	const locationValue = watch("location") ?? "";
	const websiteValue = watch("website_url") ?? "";
	const isSaving = updateProfile.isPending || upload.isPending;
	const hasChanges = isDirty || !!pendingAvatar || !!pendingBanner;

	const displayName = displayNameOf(user, "?");
	const currentBanner = bannerPreview ?? user.header_image_url ?? null;
	const currentAvatar = avatarPreview ?? user.avatar_url ?? null;

	return (
		<Dialog open onOpenChange={(o) => !o && onClose()}>
			<DialogContent>
				{/* Header */}
				<div className="flex items-center justify-between border-b border-border px-4 py-3">
					<div className="flex items-center gap-6">
						<DialogClose
							className="cursor-pointer rounded-full p-1.5 transition-colors hover:bg-muted"
							aria-label="Close"
						>
							<X className="h-5 w-5" />
						</DialogClose>
						<DialogTitle className="text-xl font-extrabold">
							Edit profile
						</DialogTitle>
					</div>
					<Button
						size="sm"
						disabled={!hasChanges || isSaving}
						onClick={handleSubmit(onSubmit)}
						className="rounded-full px-5 font-bold"
					>
						{isSaving ? "Saving…" : "Save"}
					</Button>
				</div>

				{/* Scrollable body */}
				<div className="max-h-[calc(100vh-120px)] overflow-y-auto">
					{/* Banner + Avatar */}
					<div className="relative">
						{/* Banner */}
						<div
							className="relative h-[150px] cursor-pointer bg-banner group"
							onClick={() => bannerInputRef.current?.click()}
						>
							{currentBanner ? (
								// eslint-disable-next-line @next/next/no-img-element
								<img
									src={currentBanner}
									alt="Banner"
									className="h-full w-full object-cover"
								/>
							) : (
								<div className="h-full w-full bg-banner" />
							)}
							<div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity">
								<Camera className="h-8 w-8 text-white" />
							</div>
						</div>

						{/* Avatar overlapping banner */}
						<div className="absolute left-4 bottom-0 translate-y-1/2">
							<div
								className="relative cursor-pointer rounded-full border-4 border-background group"
								onClick={() => avatarInputRef.current?.click()}
							>
								<Avatar src={currentAvatar} name={displayName} size="lg" />
								<div className="absolute inset-0 flex items-center justify-center rounded-full bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity">
									<Camera className="h-6 w-6 text-white" />
								</div>
							</div>
						</div>
					</div>

					{/* Spacer for avatar overlap */}
					<div className="mt-14 px-4 pb-5 space-y-4">
						<FloatedField
							id="display_name"
							label="Name"
							error={errors.display_name?.message}
							maxLength={50}
							currentLength={displayNameValue.length}
							registerReturn={register("display_name")}
						/>
						<FloatedField
							id="bio"
							label="Bio"
							multiline
							error={errors.bio?.message}
							maxLength={160}
							currentLength={bioValue.length}
							registerReturn={register("bio")}
						/>
						<FloatedField
							id="location"
							label="Location"
							error={errors.location?.message}
							maxLength={30}
							currentLength={locationValue.length}
							registerReturn={register("location")}
						/>
						<FloatedField
							id="website_url"
							label="Website"
							error={errors.website_url?.message}
							maxLength={100}
							currentLength={websiteValue.length}
							registerReturn={register("website_url")}
						/>
					</div>
				</div>

				{/* Hidden file inputs */}
				<input
					ref={avatarInputRef}
					type="file"
					accept="image/jpeg,image/png,image/webp"
					className="hidden"
					onChange={handleAvatarChange}
				/>
				<input
					ref={bannerInputRef}
					type="file"
					accept="image/jpeg,image/png,image/webp"
					className="hidden"
					onChange={handleBannerChange}
				/>
			</DialogContent>
		</Dialog>
	);
}
