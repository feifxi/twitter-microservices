import { redirect } from "next/navigation";
import { requireUserId } from "@/lib/auth";
import { serverGet } from "@/lib/server-api";
import type { User } from "@/lib/types";
import { OnboardingForm } from "./OnboardingForm";

export default async function OnboardingPage() {
	const userId = await requireUserId();

	let alreadyOnboarded = false;
	try {
		const user = await serverGet<User>(`/v1/users/me`);
		alreadyOnboarded = !!user.username;
	} catch {
		// Can't verify — show onboarding form
	}
	if (alreadyOnboarded) redirect("/home");

	return (
		<div className="flex min-h-screen items-center justify-center bg-background px-4">
			<div className="w-full max-w-md">
				<div className="mb-8 text-center">
					<div className="mb-4 text-4xl font-bold">𝕏</div>
					<h1 className="text-2xl font-bold">Welcome! Set up your profile</h1>
					<p className="mt-2 text-sm text-muted-foreground">
						Choose a username and display name to get started.
					</p>
				</div>
				<OnboardingForm userId={userId} />
			</div>
		</div>
	);
}
