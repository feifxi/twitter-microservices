"use client";

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { Dialog, DialogBackdrop, DialogClose } from "@/components/ui/dialog";

interface ImageLightboxProps {
	src: string;
	alt?: string;
	onClose: () => void;
}

export function ImageLightbox({ src, alt, onClose }: ImageLightboxProps) {
	return (
		<Dialog open onOpenChange={(o) => !o && onClose()}>
			<DialogPrimitive.Portal>
				<DialogBackdrop className="bg-black/90 backdrop-blur-none" />
				<DialogPrimitive.Popup className="fixed inset-0 z-50 flex items-center justify-center p-4 data-ending-style:opacity-0 data-starting-style:opacity-0">
					<DialogClose
						className="absolute left-4 top-4 cursor-pointer rounded-full bg-black/40 p-2 text-white hover:bg-black/60"
						aria-label="Close image"
					>
						<X className="h-5 w-5" />
					</DialogClose>
					{/* eslint-disable-next-line @next/next/no-img-element */}
					<img
						src={src}
						alt={alt ?? "Image preview"}
						onClick={(e) => e.stopPropagation()}
						className="max-h-full max-w-full rounded-lg object-contain"
					/>
				</DialogPrimitive.Popup>
			</DialogPrimitive.Portal>
		</Dialog>
	);
}
