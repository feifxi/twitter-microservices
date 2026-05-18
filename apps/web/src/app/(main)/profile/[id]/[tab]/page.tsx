import { notFound } from "next/navigation";
import { FollowConnectionsClient } from "@/components/FollowConnectionsClient";
import {
	type ProfileTab,
	ProfileTabContent,
} from "@/components/ProfileTabContent";
import { requireUserId } from "@/lib/auth";

// Tweet-feed tabs ("posts" lives at the bare /profile/[id], not under this segment).
const TWEET_TABS = ["replies", "media", "likes"] as const;
type TweetTab = (typeof TWEET_TABS)[number];

const CONNECTION_TABS = ["followers", "following"] as const;
type ConnectionTab = (typeof CONNECTION_TABS)[number];

function isTweetTab(value: string): value is TweetTab {
	return (TWEET_TABS as readonly string[]).includes(value);
}

function isConnectionTab(value: string): value is ConnectionTab {
	return (CONNECTION_TABS as readonly string[]).includes(value);
}

export default async function ProfileTabPage({
	params,
}: {
	params: Promise<{ id: string; tab: string }>;
}) {
	const { id, tab } = await params;
	const userId = await requireUserId();

	if (isConnectionTab(tab)) {
		// ProfileShell (in the layout) bypasses its own chrome for these segments
		// — FollowConnectionsClient renders its own sticky header.
		return (
			<FollowConnectionsClient
				profileId={id}
				currentUserId={userId}
				initialTab={tab}
			/>
		);
	}

	if (!isTweetTab(tab)) notFound();

	const profileTab: ProfileTab = tab;
	return (
		<ProfileTabContent userId={id} currentUserId={userId} tab={profileTab} />
	);
}
