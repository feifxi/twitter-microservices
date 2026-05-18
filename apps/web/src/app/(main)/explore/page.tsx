import { Suspense } from "react";
import { ExploreClient } from "@/components/ExploreClient";
import { requireUserId } from "@/lib/auth";

export default async function ExplorePage({
	searchParams,
}: {
	searchParams: Promise<{ q?: string }>;
}) {
	const userId = await requireUserId();
	const { q = "" } = await searchParams;

	return (
		<Suspense>
			<ExploreClient currentUserId={userId} initialQ={q} />
		</Suspense>
	);
}
