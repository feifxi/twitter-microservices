"use client";

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

const Dialog = DialogPrimitive.Root;
const DialogTrigger = DialogPrimitive.Trigger;
const DialogPortal = DialogPrimitive.Portal;
const DialogClose = DialogPrimitive.Close;
const DialogTitle = DialogPrimitive.Title;
const DialogDescription = DialogPrimitive.Description;

function DialogBackdrop({
	className,
	...props
}: DialogPrimitive.Backdrop.Props) {
	return (
		<DialogPrimitive.Backdrop
			className={cn(
				"fixed inset-0 z-50 bg-black/60 backdrop-blur-sm",
				"transition-opacity duration-200",
				"data-starting-style:opacity-0 data-ending-style:opacity-0",
				className,
			)}
			{...props}
		/>
	);
}

interface DialogContentProps extends DialogPrimitive.Popup.Props {
	position?: "center" | "top";
	showClose?: boolean;
}

function DialogContent({
	className,
	children,
	position = "center",
	showClose = false,
	...props
}: DialogContentProps) {
	return (
		<DialogPortal>
			<DialogBackdrop />
			<DialogPrimitive.Popup
				className={cn(
					"fixed left-1/2 z-50 w-full max-w-lg -translate-x-1/2",
					"overflow-hidden rounded-2xl bg-background shadow-xl",
					"transition-all duration-200",
					"data-starting-style:opacity-0 data-starting-style:scale-95",
					"data-ending-style:opacity-0 data-ending-style:scale-95",
					position === "center" && "top-1/2 -translate-y-1/2",
					position === "top" && "top-16",
					className,
				)}
				{...props}
			>
				{children}
				{showClose && (
					<DialogClose
						className="absolute right-3 top-3 cursor-pointer rounded-full p-1.5 transition-colors hover:bg-muted"
						aria-label="Close"
					>
						<X className="h-5 w-5" />
					</DialogClose>
				)}
			</DialogPrimitive.Popup>
		</DialogPortal>
	);
}

export {
	Dialog,
	DialogBackdrop,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogPortal,
	DialogTitle,
	DialogTrigger,
};
