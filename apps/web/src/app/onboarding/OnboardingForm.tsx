"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { useUpdateProfile } from "@/hooks/useUser";
import { type UsernameInput, usernameSchema } from "@/lib/schemas";

interface OnboardingFormProps {
	userId: string;
}

export function OnboardingForm({ userId }: OnboardingFormProps) {
	const router = useRouter();
	const updateProfile = useUpdateProfile(userId);

	const {
		register,
		handleSubmit,
		formState: { errors },
	} = useForm<UsernameInput>({
		resolver: zodResolver(usernameSchema),
	});

	async function onSubmit(data: UsernameInput) {
		await updateProfile.mutateAsync(data);
		router.push("/home");
	}

	return (
		<form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
			<div>
				<label
					htmlFor="display_name"
					className="mb-1.5 block text-sm font-medium"
				>
					Display name
				</label>
				<input
					id="display_name"
					{...register("display_name")}
					placeholder="Your name"
					className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
				/>
				{errors.display_name && (
					<p className="mt-1 text-xs text-destructive">
						{errors.display_name.message}
					</p>
				)}
			</div>

			<div>
				<label htmlFor="username" className="mb-1.5 block text-sm font-medium">
					Username
				</label>
				<div className="relative">
					<span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
						@
					</span>
					<input
						id="username"
						{...register("username")}
						placeholder="username"
						className="w-full rounded-lg border border-input bg-background py-2 pl-7 pr-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
					/>
				</div>
				{errors.username && (
					<p className="mt-1 text-xs text-destructive">
						{errors.username.message}
					</p>
				)}
			</div>

			{updateProfile.isError && (
				<p className="text-sm text-destructive">
					Something went wrong. Please try again.
				</p>
			)}

			<Button
				type="submit"
				className="w-full rounded-full font-bold"
				disabled={updateProfile.isPending}
			>
				{updateProfile.isPending ? "Saving…" : "Get started"}
			</Button>
		</form>
	);
}
