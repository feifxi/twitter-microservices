"use client";

import { useEffect, useState } from "react";

// Returns `value` only after it has stayed unchanged for `delay` ms.
// Use for "fire API on user input" patterns (e.g. search-as-you-type) so
// every keystroke doesn't trigger a request — only the value the user
// settled on does.
export function useDebouncedValue<T>(value: T, delay = 300): T {
	const [debounced, setDebounced] = useState(value);

	useEffect(() => {
		const t = setTimeout(() => setDebounced(value), delay);
		return () => clearTimeout(t);
	}, [value, delay]);

	return debounced;
}
