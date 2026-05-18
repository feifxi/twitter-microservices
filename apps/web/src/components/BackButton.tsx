"use client";

import { ArrowLeft } from "lucide-react";
import { useRouter } from "next/navigation";

export function BackButton() {
	const router = useRouter();
	return (
		<button
			onClick={() => router.back()}
			className="rounded-full p-2 transition-colors hover:bg-muted"
			aria-label="Back"
		>
			<ArrowLeft className="h-5 w-5" />
		</button>
	);
}
