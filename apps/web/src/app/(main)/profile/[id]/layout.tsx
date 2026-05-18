import {
	dehydrate,
	HydrationBoundary,
	QueryClient,
} from "@tanstack/react-query";
import { ProfileShell } from "@/components/ProfileShell";
import { requireUserId } from "@/lib/auth";
import { keys } from "@/lib/query-keys";
import { serverGet } from "@/lib/server-api";
import type { User } from "@/lib/types";

// Shared layout for /profile/[id]/* — Next caches layouts across navigation,
// so the banner/avatar/bio/tab strip don't remount when switching tabs.
// ProfileShell is a thin client wrapper that reads the active segment to
// decide whether to render the profile chrome (posts/replies/media/likes)
// or step out of the way for followers/following, which bring their own.
export default async function ProfileLayout({
	children,
	params,
}: {
	children: React.ReactNode;
	params: Promise<{ id: string }>;
}) {
	const { id } = await params;
	const userId = await requireUserId();
	const queryClient = new QueryClient();

	let user: User | null = null;
	try {
		user = await serverGet<User>(`/v1/users/${id}`);
		queryClient.setQueryData(keys.users.detail(id), user);
	} catch {
		// Non-fatal — ProfileShell renders a not-found state
	}

	return (
		<HydrationBoundary state={dehydrate(queryClient)}>
			<ProfileShell user={user} currentUserId={userId}>
				{children}
			</ProfileShell>
		</HydrationBoundary>
	);
}
