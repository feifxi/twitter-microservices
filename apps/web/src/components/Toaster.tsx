"use client";

import { Toast } from "@base-ui/react/toast";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import { toastManager } from "@/store/toast";

export function Toaster() {
	return (
		<Toast.Provider toastManager={toastManager}>
			<Toast.Portal>
				<Toast.Viewport className="fixed bottom-6 left-1/2 z-200 flex -translate-x-1/2 flex-col items-center gap-2">
					<ToastList />
				</Toast.Viewport>
			</Toast.Portal>
		</Toast.Provider>
	);
}

function ToastList() {
	const { toasts } = Toast.useToastManager();

	return toasts.map((t) => (
		<Toast.Root
			key={t.id}
			toast={t}
			className={cn(
				"flex min-w-[280px] max-w-sm items-center justify-between gap-3 rounded-full px-5 py-3 text-[15px] font-medium shadow-lg",
				"transition-all duration-200",
				"data-starting-style:translate-y-2 data-starting-style:opacity-0",
				"data-ending-style:opacity-0",
				t.type === "error"
					? "bg-destructive text-white"
					: "bg-primary text-primary-foreground",
			)}
		>
			<Toast.Title>{t.title}</Toast.Title>
			<Toast.Close
				className="shrink-0 rounded-full p-0.5 opacity-70 hover:opacity-100"
				aria-label="Dismiss"
			>
				<X className="h-4 w-4" />
			</Toast.Close>
		</Toast.Root>
	));
}
