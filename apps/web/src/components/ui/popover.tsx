"use client";

import { Popover as PopoverPrimitive } from "@base-ui/react/popover";
import { cn } from "@/lib/utils";

const Popover = PopoverPrimitive.Root;
const PopoverTrigger = PopoverPrimitive.Trigger;
const PopoverPortal = PopoverPrimitive.Portal;
const PopoverClose = PopoverPrimitive.Close;

function PopoverContent({
	className,
	side = "bottom",
	align = "start",
	sideOffset = 8,
	children,
	...props
}: PopoverPrimitive.Positioner.Props & { children?: React.ReactNode }) {
	return (
		<PopoverPortal>
			<PopoverPrimitive.Positioner
				side={side}
				align={align}
				sideOffset={sideOffset}
				{...props}
			>
				<PopoverPrimitive.Popup
					className={cn(
						"z-50 outline-none",
						"transition-opacity duration-150",
						"data-starting-style:opacity-0 data-ending-style:opacity-0",
						className,
					)}
				>
					{children}
				</PopoverPrimitive.Popup>
			</PopoverPrimitive.Positioner>
		</PopoverPortal>
	);
}

export { Popover, PopoverClose, PopoverContent, PopoverPortal, PopoverTrigger };
