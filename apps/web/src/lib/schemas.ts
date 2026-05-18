import { z } from "zod";

// Body OR media is required — that constraint is enforced by the composer's
// submit-disabled check (and by the backend). zod only caps the upper bound.
export const tweetSchema = z.object({
	body: z.string().max(280, "Tweet cannot exceed 280 characters"),
	media_id: z.string().optional(),
	media_url: z.string().optional(),
});

export const profileSchema = z.object({
	display_name: z
		.string()
		.min(1, "Display name cannot be empty")
		.max(50, "Display name cannot exceed 50 characters"),
	bio: z.string().max(160, "Bio cannot exceed 160 characters").optional(),
	location: z
		.string()
		.max(30, "Location cannot exceed 30 characters")
		.optional(),
	website_url: z
		.string()
		.max(100, "Website cannot exceed 100 characters")
		.refine(
			(v) => v === "" || /^https?:\/\/.+/.test(v),
			"Must be a valid URL (https://...)",
		)
		.optional(),
	avatar_url: z.string().optional(),
	header_image_url: z.string().optional(),
});

export const usernameSchema = z.object({
	username: z
		.string()
		.min(3, "Username must be at least 3 characters")
		.max(20, "Username cannot exceed 20 characters")
		.regex(
			/^[a-zA-Z0-9_]+$/,
			"Username can only contain letters, numbers, and underscores",
		),
	display_name: z
		.string()
		.min(1, "Display name cannot be empty")
		.max(50, "Display name cannot exceed 50 characters"),
});

export type TweetInput = z.infer<typeof tweetSchema>;
export type ProfileInput = z.infer<typeof profileSchema>;
export type UsernameInput = z.infer<typeof usernameSchema>;
