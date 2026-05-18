import { NotificationsClient } from "@/components/NotificationsClient";
import { requireUserId } from "@/lib/auth";

export default async function NotificationsPage() {
	await requireUserId();
	return <NotificationsClient />;
}
