"use client";

import {
	AlertDialog,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";

interface ConfirmModalProps {
	title: string;
	description: string;
	confirmLabel: string;
	cancelLabel?: string;
	danger?: boolean;
	loading?: boolean;
	onConfirm: () => void;
	onClose: () => void;
}

export function ConfirmModal({
	title,
	description,
	confirmLabel,
	cancelLabel = "Cancel",
	danger = false,
	loading = false,
	onConfirm,
	onClose,
}: ConfirmModalProps) {
	return (
		<AlertDialog open onOpenChange={(o) => !o && !loading && onClose()}>
			<AlertDialogContent>
				<AlertDialogTitle className="text-[23px] font-extrabold leading-tight">
					{title}
				</AlertDialogTitle>
				<AlertDialogDescription className="mt-2 text-[15px] text-muted-foreground">
					{description}
				</AlertDialogDescription>

				<div className="mt-6 flex flex-col gap-3">
					<button
						type="button"
						onClick={onConfirm}
						disabled={loading}
						className={cn(
							"w-full cursor-pointer rounded-full py-3 text-[17px] font-bold transition-all duration-150 hover:opacity-90 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-60",
							danger
								? "bg-destructive text-white"
								: "bg-foreground text-background",
						)}
					>
						{loading ? "..." : confirmLabel}
					</button>
					<button
						type="button"
						onClick={onClose}
						disabled={loading}
						className="w-full cursor-pointer rounded-full border border-border py-3 text-[17px] font-bold transition-all duration-150 hover:bg-muted active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-60"
					>
						{cancelLabel}
					</button>
				</div>
			</AlertDialogContent>
		</AlertDialog>
	);
}
