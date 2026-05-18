"use client";

import { TweetComposer } from "@/components/TweetComposer";
import { Dialog, DialogContent } from "@/components/ui/dialog";

interface PostComposerModalProps {
	currentUserId: string;
	onClose: () => void;
}

export function PostComposerModal({
	currentUserId,
	onClose,
}: PostComposerModalProps) {
	return (
		<Dialog open onOpenChange={(o) => !o && onClose()}>
			<DialogContent position="top" showClose className="max-w-xl">
				<TweetComposer currentUserId={currentUserId} onSuccess={onClose} />
			</DialogContent>
		</Dialog>
	);
}
