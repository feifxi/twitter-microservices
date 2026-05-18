"use client";

import { AlertDialog as AlertDialogPrimitive } from "@base-ui/react/alert-dialog";
import { cn } from "@/lib/utils";

const AlertDialog = AlertDialogPrimitive.Root;
const AlertDialogTrigger = AlertDialogPrimitive.Trigger;
const AlertDialogPortal = AlertDialogPrimitive.Portal;
const AlertDialogClose = AlertDialogPrimitive.Close;
const AlertDialogTitle = AlertDialogPrimitive.Title;
const AlertDialogDescription = AlertDialogPrimitive.Description;

function AlertDialogBackdrop({
	className,
	...props
}: AlertDialogPrimitive.Backdrop.Props) {
	return (
		<AlertDialogPrimitive.Backdrop
			className={cn(
				"fixed inset-0 z-150 bg-black/50",
				"transition-opacity duration-200",
				"data-starting-style:opacity-0 data-ending-style:opacity-0",
				className,
			)}
			{...props}
		/>
	);
}

function AlertDialogContent({
	className,
	children,
	...props
}: AlertDialogPrimitive.Popup.Props) {
	return (
		<AlertDialogPortal>
			<AlertDialogBackdrop />
			<AlertDialogPrimitive.Popup
				className={cn(
					"fixed left-1/2 top-1/2 z-150 w-full max-w-[320px]",
					"-translate-x-1/2 -translate-y-1/2",
					"rounded-2xl bg-background p-8 shadow-2xl",
					"transition-all duration-200",
					"data-starting-style:opacity-0 data-starting-style:scale-95",
					"data-ending-style:opacity-0 data-ending-style:scale-95",
					className,
				)}
				{...props}
			>
				{children}
			</AlertDialogPrimitive.Popup>
		</AlertDialogPortal>
	);
}

export {
	AlertDialog,
	AlertDialogBackdrop,
	AlertDialogClose,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogPortal,
	AlertDialogTitle,
	AlertDialogTrigger,
};
