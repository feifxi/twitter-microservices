import { MessageCircle } from "lucide-react";
import { requireUserId } from "@/lib/auth";

export default async function MessagesPage() {
	await requireUserId();

	return (
		<div className="flex h-full flex-col">
			<div className="sticky top-0 z-10 border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<h1 className="text-xl font-extrabold">Chat</h1>
			</div>

			<div className="flex flex-1 flex-col items-center px-8 py-16 text-center">
				<div className="mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
					<MessageCircle className="h-8 w-8 text-primary" />
				</div>
				<h2 className="mb-2 text-3xl font-extrabold">Welcome to your inbox!</h2>
				<p className="max-w-xs text-[17px] text-muted-foreground">
					Send and receive direct messages with the people who matter to you.
				</p>
				<p className="mt-6 text-sm text-muted-foreground">
					Not available in this build.
				</p>
			</div>
		</div>
	);
}
