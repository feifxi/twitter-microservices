"use client";

import { RefreshCw, Users } from "lucide-react";
import { UserRow, UserRowSkeleton } from "@/components/UserRow";
import { useSuggestions } from "@/hooks/useUser";

interface ConnectClientProps {
	currentUserId: string;
}

export function ConnectClient({ currentUserId }: ConnectClientProps) {
	const suggestions = useSuggestions(30);

	return (
		<div>
			<div className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-background/80 px-4 py-3 backdrop-blur-sm">
				<div>
					<h1 className="text-xl font-extrabold">Follow</h1>
					<p className="text-sm text-muted-foreground">Suggested for you</p>
				</div>
				<button
					type="button"
					onClick={() => suggestions.refetch()}
					disabled={suggestions.isFetching}
					aria-label="Refresh suggestions"
					className="cursor-pointer rounded-full p-2 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
				>
					<RefreshCw
						className={`h-5 w-5 ${suggestions.isFetching ? "animate-spin" : ""}`}
					/>
				</button>
			</div>

			{suggestions.isLoading && (
				<>
					{Array.from({ length: 6 }).map((_, i) => (
						<UserRowSkeleton key={i} />
					))}
				</>
			)}

			{suggestions.isError && (
				<div className="px-4 py-12 text-center">
					<p className="text-[15px] text-muted-foreground">
						Failed to load suggestions.
					</p>
					<button
						type="button"
						onClick={() => suggestions.refetch()}
						className="mt-2 cursor-pointer text-[15px] font-medium text-primary hover:underline"
					>
						Try again
					</button>
				</div>
			)}

			{!suggestions.isLoading && suggestions.data?.length === 0 && (
				<div className="flex flex-col items-center px-8 py-16 text-center">
					<div className="mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
						<Users className="h-8 w-8 text-primary" />
					</div>
					<h2 className="mb-2 text-3xl font-extrabold">
						No one to suggest yet
					</h2>
					<p className="max-w-xs text-[17px] text-muted-foreground">
						Once there are more accounts on the platform, you'll see suggestions
						here.
					</p>
				</div>
			)}

			{suggestions.data?.map((u) => (
				<UserRow key={u.id} user={u} isSelf={u.id === currentUserId} />
			))}
		</div>
	);
}
