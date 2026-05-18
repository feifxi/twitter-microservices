import { redirect } from "next/navigation";
import { NotificationListener } from "@/components/NotificationListener";
import { Sidebar } from "@/components/Sidebar";
import { Toaster } from "@/components/Toaster";
import { TrendingSidebar } from "@/components/TrendingSidebar";
import { requireUserId } from "@/lib/auth";
import { serverGet } from "@/lib/server-api";
import type { User } from "@/lib/types";

export default async function MainLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	const userId = await requireUserId();

	let needsOnboarding = false;
	try {
		const user = await serverGet<User>(`/v1/users/me`);
		needsOnboarding = !user.username;
	} catch {
		// Non-fatal — don't block if user-service is down
	}
	if (needsOnboarding) redirect("/onboarding");

	return (
		<div className="min-h-screen bg-background">
			<div className="mx-auto flex max-w-7xl">
				<aside className="sticky top-0 h-screen w-16 shrink-0 xl:w-[275px]">
					<Sidebar userId={userId} />
				</aside>

				<main className="min-h-screen flex-1 border-x border-border">
					<NotificationListener />
					{children}
				</main>

				<Toaster />

				<aside className="sticky top-0 hidden h-screen w-[350px] shrink-0 lg:block">
					<TrendingSidebar />
				</aside>
			</div>
		</div>
	);
}
