import Link from "next/link";

// Splits a tweet body on hashtags and renders each #tag as a link to explore.
// Pass stopPropagation=true when rendered inside a clickable card so the tag
// click doesn't bubble up to the card's onClick.
export function renderTweetBody(
	content: string,
	stopPropagation = false,
): React.ReactNode[] {
	return content.split(/(#\w+)/g).map((part, i) => {
		if (!/^#\w+$/.test(part)) return part;
		return (
			<Link
				// biome-ignore lint/suspicious/noArrayIndexKey: split order is stable for a given body
				key={i}
				href={`/explore?q=${encodeURIComponent(part)}`}
				onClick={stopPropagation ? (e) => e.stopPropagation() : undefined}
				className="text-primary hover:underline"
			>
				{part}
			</Link>
		);
	});
}
