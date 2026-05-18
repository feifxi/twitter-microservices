"use client";

import {
	Bell,
	Bookmark,
	Ellipsis,
	Home,
	LogOut,
	MessageCircle,
	PenLine,
	Search,
	User,
	Users,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { Avatar } from "@/components/Avatar";
import { ConfirmModal } from "@/components/ConfirmModal";
import { PostComposerModal } from "@/components/PostComposerModal";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useProfile } from "@/hooks/useUser";
import { cn, displayNameOf } from "@/lib/utils";
import { useNotificationStore } from "@/store/notification";

interface NavItem {
	href: string;
	icon: React.ElementType;
	label: string;
	showBadge?: boolean;
}

interface SidebarProps {
	userId: string;
}

function XLogo() {
	return (
		<svg
			viewBox="0 0 24 24"
			aria-hidden="true"
			className="h-7 w-7 fill-current"
		>
			<path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-4.714-6.231-5.401 6.231H2.747l7.73-8.835L1.254 2.25H8.08l4.253 5.622zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
		</svg>
	);
}

export function Sidebar({ userId }: SidebarProps) {
	const pathname = usePathname();
	const notificationCount = useNotificationStore((s) => s.count);
	const [composerOpen, setComposerOpen] = useState(false);
	const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false);
	const { data: me } = useProfile(userId);

	const navItems: NavItem[] = [
		{ href: "/home", icon: Home, label: "Home" },
		{ href: "/explore", icon: Search, label: "Explore" },
		{
			href: "/notifications",
			icon: Bell,
			label: "Notifications",
			showBadge: true,
		},
		{ href: "/connect", icon: Users, label: "Follow" },
		{ href: "/messages", icon: MessageCircle, label: "Chat" },
		{ href: "/bookmarks", icon: Bookmark, label: "Bookmarks" },
		{ href: `/profile/${userId}`, icon: User, label: "Profile" },
	];

	const displayName = displayNameOf(me, "You");
	const username = me?.username ?? null;

	function handleLogoutConfirm() {
		window.location.href = "/api/auth/logout";
	}

	return (
		<>
			<nav className="flex h-full flex-col items-center px-2 py-2 xl:items-start xl:px-3">
				{/* X Logo */}
				<Link
					href="/home"
					className="mb-1 flex h-[52px] w-[52px] cursor-pointer items-center justify-center rounded-full transition-colors hover:bg-muted"
					aria-label="Home"
				>
					<XLogo />
				</Link>

				{/* Nav links */}
				<div className="flex flex-1 flex-col gap-0.5">
					{navItems.map(({ href, icon: Icon, label, showBadge }) => {
						const isActive =
							pathname === href || pathname.startsWith(href + "/");
						const badge = showBadge && notificationCount > 0;

						return (
							<Link
								key={href}
								href={href}
								className={cn(
									"group flex items-center gap-4 rounded-full px-3 py-3 text-[1.25rem] transition-colors hover:bg-muted xl:pr-5",
									isActive && "font-bold",
								)}
								aria-label={label}
							>
								<span className="relative shrink-0">
									<Icon
										className="h-6.5 w-6.5"
										strokeWidth={isActive ? 2.5 : 2}
									/>
									{badge && (
										<span className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-bold text-primary-foreground">
											{notificationCount > 99 ? "99+" : notificationCount}
										</span>
									)}
								</span>
								<span className="hidden xl:block">{label}</span>
							</Link>
						);
					})}
				</div>

				{/* Post button */}
				<button
					type="button"
					onClick={() => setComposerOpen(true)}
					className="mb-3 mt-1 hidden w-full cursor-pointer items-center justify-center rounded-full bg-primary px-4 py-3 text-[1.0625rem] font-bold text-primary-foreground transition-all duration-150 hover:opacity-90 active:scale-95 xl:flex"
				>
					Post
				</button>
				<button
					type="button"
					onClick={() => setComposerOpen(true)}
					aria-label="Compose post"
					className="mb-3 mt-1 flex h-[52px] w-[52px] cursor-pointer items-center justify-center rounded-full bg-primary text-primary-foreground transition-all duration-150 hover:opacity-90 active:scale-95 xl:hidden"
				>
					<PenLine className="h-6 w-6" />
				</button>

				{/* Account section */}
				<DropdownMenu>
					<DropdownMenuTrigger
						render={
							<button
								type="button"
								className="flex w-full cursor-pointer items-center gap-3 rounded-full px-3 py-3 text-left transition-colors hover:bg-muted xl:justify-between"
							>
								<div className="flex min-w-0 items-center gap-3">
									<Avatar
										src={me?.avatar_url ?? null}
										name={displayName}
										size="sm"
									/>
									<div className="hidden min-w-0 xl:block">
										<p className="truncate text-sm font-bold leading-tight">
											{displayName}
										</p>
										{username && (
											<p className="truncate text-sm text-muted-foreground leading-tight">
												@{username}
											</p>
										)}
									</div>
								</div>
								<Ellipsis className="hidden h-5 w-5 shrink-0 text-muted-foreground xl:block" />
							</button>
						}
					/>
					<DropdownMenuContent
						side="top"
						align="start"
						sideOffset={8}
						className="w-[260px]"
					>
						<DropdownMenuItem onClick={() => setLogoutConfirmOpen(true)}>
							<LogOut className="h-4 w-4 shrink-0 text-muted-foreground" />
							Log out {username ? `@${username}` : ""}
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</nav>

			{composerOpen && (
				<PostComposerModal
					currentUserId={userId}
					onClose={() => setComposerOpen(false)}
				/>
			)}

			{logoutConfirmOpen && (
				<ConfirmModal
					title="Log out?"
					description="You can always log back in at any time. If you just want to switch accounts, you can do that by adding an existing account."
					confirmLabel="Log out"
					onConfirm={handleLogoutConfirm}
					onClose={() => setLogoutConfirmOpen(false)}
				/>
			)}
		</>
	);
}
