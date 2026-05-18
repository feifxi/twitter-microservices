import { type ClassValue, clsx } from "clsx";
import {
	differenceInDays,
	differenceInHours,
	differenceInMinutes,
	differenceInSeconds,
	format,
} from "date-fns";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

// Deterministic name → RGB color for fallback avatars. Returns "rgb(r, g, b)"
// rather than hsl() so the string is identical on server and client — some
// CSS pipelines normalize hsl() → rgb() during SSR, causing hydration drift.
export function avatarColorFromName(name: string): string {
	let hash = 0;
	for (let i = 0; i < name.length; i++) {
		hash = (hash << 5) - hash + name.charCodeAt(i);
		hash |= 0;
	}
	const h = Math.abs(hash) % 360;
	const s = 0.65;
	const l = 0.55;
	const c = (1 - Math.abs(2 * l - 1)) * s;
	const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
	const m = l - c / 2;
	let r = 0;
	let g = 0;
	let b = 0;
	if (h < 60) [r, g, b] = [c, x, 0];
	else if (h < 120) [r, g, b] = [x, c, 0];
	else if (h < 180) [r, g, b] = [0, c, x];
	else if (h < 240) [r, g, b] = [0, x, c];
	else if (h < 300) [r, g, b] = [x, 0, c];
	else [r, g, b] = [c, 0, x];
	return `rgb(${Math.round((r + m) * 255)}, ${Math.round((g + m) * 255)}, ${Math.round((b + m) * 255)})`;
}

// Compact display for engagement counts. hideZero=true returns "" for 0
// (used in action buttons where empty space reads cleaner than "0").
export function formatCount(n: number, hideZero = false): string {
	if (n === 0 && hideZero) return "";
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
	return n.toString();
}

export function formatScore(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M posts`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K posts`;
	return `${n} posts`;
}

interface NameLike {
	display_name?: string | null;
	username?: string | null;
}

// Single source of truth for "what name do we show for this user".
// Uses `||` (not `??`) so empty strings — which the Go backend returns instead
// of null for unset fields — fall through to username/fallback rather than
// rendering as blank.
export function displayNameOf(
	user: NameLike | null | undefined,
	fallback = "Unknown",
): string {
	return user?.display_name || user?.username || fallback;
}

export function formatTime(dateStr: string): string {
	const date = new Date(dateStr);
	const now = new Date();
	const seconds = differenceInSeconds(now, date);
	if (seconds < 60) return `${seconds}s`;
	const minutes = differenceInMinutes(now, date);
	if (minutes < 60) return `${minutes}m`;
	const hours = differenceInHours(now, date);
	if (hours < 24) return `${hours}h`;
	const days = differenceInDays(now, date);
	if (days < 7) return `${days}d`;
	return format(date, "MMM d");
}
