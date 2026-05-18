import { Avatar as AvatarPrimitive } from "@base-ui/react/avatar";
import { avatarColorFromName, cn } from "@/lib/utils";

interface AvatarProps {
	src: string | null;
	name: string;
	size?: "sm" | "md" | "lg";
}

export function Avatar({ src, name, size = "md" }: AvatarProps) {
	const initials = name
		.split(" ")
		.map((w) => w[0])
		.join("")
		.slice(0, 2)
		.toUpperCase();

	const sizeClass =
		size === "sm"
			? "h-8 w-8 text-xs"
			: size === "lg"
				? "h-24 w-24 text-2xl font-bold"
				: "h-10 w-10 text-sm";

	const bg = avatarColorFromName(name);

	return (
		<AvatarPrimitive.Root
			className={cn(
				"inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full font-semibold text-white",
				sizeClass,
			)}
			style={{ backgroundColor: bg }}
		>
			{src && (
				<AvatarPrimitive.Image
					src={src}
					alt={name}
					className="h-full w-full object-cover"
				/>
			)}
			<AvatarPrimitive.Fallback delay={0}>
				{initials || "?"}
			</AvatarPrimitive.Fallback>
		</AvatarPrimitive.Root>
	);
}
