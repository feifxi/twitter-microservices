import { ProfileTabContent } from "@/components/ProfileTabContent";
import { requireUserId } from "@/lib/auth";

export default async function ProfilePage({
	params,
}: {
	params: Promise<{ id: string }>;
}) {
	const { id } = await params;
	const userId = await requireUserId();
	return <ProfileTabContent userId={id} currentUserId={userId} tab="posts" />;
}
