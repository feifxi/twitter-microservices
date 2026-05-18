"use client";

import Link from "next/link";
import { cn, formatCount } from "@/lib/utils";

type ActionColor = "default" | "green" | "pink";

interface ColorTokens {
	active: string;
	hoverBg: string;
	hoverText: string;
}

// Brand colors come from CSS tokens in globals.css.
const COLORS: Record<ActionColor, ColorTokens> = {
	default: {
		active: "text-primary",
		hoverBg: "group-hover:bg-primary/10",
		hoverText: "group-hover:text-primary",
	},
	green: {
		active: "text-retweet",
		hoverBg: "group-hover:bg-retweet/10",
		hoverText: "group-hover:text-retweet",
	},
	pink: {
		active: "text-like",
		hoverBg: "group-hover:bg-like/10",
		hoverText: "group-hover:text-like",
	},
};

interface TweetActionButtonProps {
	icon: React.ReactNode;
	label: string;
	count?: number;
	active?: boolean;
	color?: ActionColor;
	size?: "sm" | "md";
	onClick?: (e: React.MouseEvent) => void;
	asLink?: string;
}

export function TweetActionButton({
	icon,
	label,
	count,
	active,
	color = "default",
	size = "sm",
	onClick,
	asLink,
}: TweetActionButtonProps) {
	const tokens = COLORS[color];
	const padding = size === "sm" ? "p-2" : "p-2.5";

	const inner = (
		<>
			<span
				className={cn(
					"rounded-full transition-all duration-150 active:scale-90",
					padding,
					tokens.hoverBg,
					tokens.hoverText,
					active && tokens.active,
				)}
			>
				{icon}
			</span>
			{count !== undefined && count > 0 && (
				<span className="min-w-[1ch] text-xs tabular-nums">
					{formatCount(count)}
				</span>
			)}
		</>
	);

	const baseClass = cn(
		"group flex cursor-pointer items-center gap-0.5 text-muted-foreground transition-colors",
		active && tokens.active,
	);

	if (asLink) {
		return (
			<Link
				href={asLink}
				onClick={(e) => e.stopPropagation()}
				className={baseClass}
				aria-label={label}
			>
				{inner}
			</Link>
		);
	}
	return (
		<button
			type="button"
			onClick={onClick}
			className={baseClass}
			aria-label={label}
		>
			{inner}
		</button>
	);
}
