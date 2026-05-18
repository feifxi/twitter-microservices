import { ConnectClient } from "@/components/ConnectClient";
import { requireUserId } from "@/lib/auth";

export default async function ConnectPage() {
	const userId = await requireUserId();
	return <ConnectClient currentUserId={userId} />;
}
