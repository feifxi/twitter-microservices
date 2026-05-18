"use client";

import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";
import { cn } from "@/lib/utils";

const Tabs = TabsPrimitive.Root;

function TabsList({ className, ...props }: TabsPrimitive.List.Props) {
	return (
		<TabsPrimitive.List
			className={cn("flex border-b border-border", className)}
			{...props}
		/>
	);
}

function TabsTab({ className, children, ...props }: TabsPrimitive.Tab.Props) {
	return (
		<TabsPrimitive.Tab
			className={cn(
				"group flex flex-1 items-center justify-center py-4 text-[15px] font-medium transition-colors outline-none hover:bg-foreground/5",
				"text-muted-foreground data-active:font-bold data-active:text-foreground",
				className,
			)}
			{...props}
		>
			<span className="relative">
				{children}
				<span className="pointer-events-none absolute -bottom-4 left-0 right-0 h-1 rounded-full bg-primary opacity-0 group-data-active:opacity-100" />
			</span>
		</TabsPrimitive.Tab>
	);
}

const TabsPanel = TabsPrimitive.Panel;

export { Tabs, TabsList, TabsPanel, TabsTab };
