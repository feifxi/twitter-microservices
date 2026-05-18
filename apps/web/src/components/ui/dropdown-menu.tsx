"use client";

import { Menu as MenuPrimitive } from "@base-ui/react/menu";
import { cn } from "@/lib/utils";

const DropdownMenu = MenuPrimitive.Root;
const DropdownMenuTrigger = MenuPrimitive.Trigger;
const DropdownMenuPortal = MenuPrimitive.Portal;

function DropdownMenuContent({
	className,
	side = "bottom",
	align = "end",
	sideOffset = 4,
	children,
	...props
}: MenuPrimitive.Positioner.Props & { children?: React.ReactNode }) {
	return (
		<DropdownMenuPortal>
			<MenuPrimitive.Positioner
				side={side}
				align={align}
				sideOffset={sideOffset}
				{...props}
			>
				{/* React events bubble through portals — stop them at the menu
				    boundary so clicking an item inside a clickable parent (like
				    a TweetCard article) doesn't trigger the parent's onClick. */}
				<MenuPrimitive.Popup
					onClick={(e) => e.stopPropagation()}
					className={cn(
						"z-50 min-w-[220px] overflow-hidden rounded-xl border border-border bg-background py-1 shadow-[0_0_20px_rgba(0,0,0,0.2)]",
						"transition-all duration-150 outline-none",
						"data-starting-style:opacity-0 data-starting-style:scale-95",
						"data-ending-style:opacity-0 data-ending-style:scale-95",
						className,
					)}
				>
					{children}
				</MenuPrimitive.Popup>
			</MenuPrimitive.Positioner>
		</DropdownMenuPortal>
	);
}

function DropdownMenuItem({ className, ...props }: MenuPrimitive.Item.Props) {
	return (
		<MenuPrimitive.Item
			className={cn(
				"flex w-full cursor-pointer items-center gap-3 px-4 py-3 text-[15px] font-bold transition-colors outline-none",
				"data-highlighted:bg-muted",
				"data-disabled:cursor-not-allowed data-disabled:font-normal data-disabled:text-muted-foreground/50",
				className,
			)}
			{...props}
		/>
	);
}

export {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuPortal,
	DropdownMenuTrigger,
};
