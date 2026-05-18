"use client";

import { useSelectedLayoutSegment } from "next/navigation";
import { BackButton } from "@/components/BackButton";
import { ProfileHeader } from "@/components/ProfileHeader";
import type { User } from "@/lib/types";
import { displayNameOf } from "@/lib/utils";

// Segments under /profile/[id]/ that render their own header (back arrow +
// title + tab strip). The shell steps out of the way so we don't stack two.
const CONNECTION_SEGMENTS = new Set(["followers", "following"]);

interface ProfileShellProps {
	user: User | null;
	currentUserId: string;
	children: React.ReactNode;
}

export function ProfileShell({
	user,
	currentUserId,
	children,
}: ProfileShellProps) {
	const segment = useSelectedLayoutSegment();

	if (segment && CONNECTION_SEGMENTS.has(segment)) {
		return <>{children}</>;
	}

	if (!user) {
		return (
			<div className="px-4 py-12 text-center text-sm text-muted-foreground">
				User not found.
			</div>
		);
	}

	const displayName = displayNameOf(user, "Profile");

	return (
		<div>
			<header className="sticky top-0 z-10 flex items-center gap-6 bg-background/80 px-4 py-3 backdrop-blur-sm">
				<BackButton />
				<h1 className="text-xl font-bold leading-tight">{displayName}</h1>
			</header>
			<ProfileHeader user={user} currentUserId={currentUserId} />
			<div className="border-t border-border">{children}</div>
		</div>
	);
}
