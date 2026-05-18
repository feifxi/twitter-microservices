"use client";

import { Flag, MoreHorizontal, Repeat2, Trash2 } from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface TweetActionsMenuProps {
	isOwn: boolean;
	isRetweeted: boolean;
	onDelete: () => void;
	onRepost: () => void;
}

export function TweetActionsMenu({
	isOwn,
	isRetweeted,
	onDelete,
	onRepost,
}: TweetActionsMenuProps) {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<button
						type="button"
						aria-label="More options"
						onClick={(e) => e.stopPropagation()}
						className="cursor-pointer rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
					>
						<MoreHorizontal className="h-4 w-4" />
					</button>
				}
			/>
			<DropdownMenuContent>
				<DropdownMenuItem onClick={onRepost}>
					<Repeat2 className="h-5 w-5 shrink-0" />
					{isRetweeted ? "Undo repost" : "Repost"}
				</DropdownMenuItem>
				{isOwn && (
					<DropdownMenuItem
						onClick={onDelete}
						className="text-destructive data-highlighted:text-destructive"
					>
						<Trash2 className="h-5 w-5 shrink-0" />
						Delete
					</DropdownMenuItem>
				)}
				<DropdownMenuItem disabled>
					<Flag className="h-5 w-5 shrink-0" />
					Report post
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
