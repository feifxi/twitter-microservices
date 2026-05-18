// Trailing debounce + state diffing for toggle mutations (like, retweet, follow).
//
// Why: optimistic UI makes the button feel instant, but if the user spam-clicks
// like→unlike→like→unlike the naive approach fires N HTTP requests that race
// each other on the server. Instead we:
//   1. Apply the cache update synchronously (UI is already instant).
//   2. Schedule a 400ms timer. If the user clicks again before it fires, we
//      replace the timer with a fresh one (intent updated).
//   3. When the timer finally fires, compare intent vs the last server-known
//      state. If they match (user toggled back to the original), send no
//      request at all. Otherwise send exactly ONE request matching final intent.
//
// State is module-level so it survives component unmount: if the user clicks
// like on the home feed and instantly navigates away, the timer still fires
// against the QueryClient (which is also a singleton).

interface PendingToggle {
	timer: ReturnType<typeof setTimeout>;
	// What the server is believed to be in. Set on first click of a debounce
	// cycle from the caller; never updated mid-cycle (the server hasn't changed
	// yet — we're still buffering).
	syncedState: boolean;
	// Latest user intent — updated on every click within the debounce window.
	intendedState: boolean;
	// Most recent sync function (closes over the latest cache helpers etc.).
	sync: (state: boolean) => Promise<void>;
}

const pending = new Map<string, PendingToggle>();

const DEFAULT_DELAY_MS = 400;

function fire(scope: string) {
	const entry = pending.get(scope);
	if (!entry) return;
	pending.delete(scope);
	// User toggled back to original state — skip the network entirely.
	if (entry.intendedState === entry.syncedState) return;
	entry.sync(entry.intendedState).catch(() => {
		// Caller is responsible for rollback / refetch via invalidateQueries
		// inside the sync function's finally. Nothing useful to do here.
	});
}

/**
 * Schedule (or reschedule) a toggle sync.
 *
 * @param scope            Unique key per toggleable resource (e.g. `like:${tweetId}`).
 * @param currentServerState  Pre-click server state. Only consulted on the first
 *                            click of a debounce cycle; ignored on subsequent
 *                            clicks since the server hasn't changed.
 * @param newIntendedState The state the user just toggled toward.
 * @param sync             Fires the actual HTTP request for the final state.
 * @param delayMs          How long to wait after the LAST click before syncing.
 */
export function debouncedToggle(
	scope: string,
	currentServerState: boolean,
	newIntendedState: boolean,
	sync: (state: boolean) => Promise<void>,
	delayMs: number = DEFAULT_DELAY_MS,
): void {
	const existing = pending.get(scope);
	if (existing) {
		clearTimeout(existing.timer);
		existing.intendedState = newIntendedState;
		existing.sync = sync;
		existing.timer = setTimeout(() => fire(scope), delayMs);
		return;
	}
	pending.set(scope, {
		timer: setTimeout(() => fire(scope), delayMs),
		syncedState: currentServerState,
		intendedState: newIntendedState,
		sync,
	});
}
