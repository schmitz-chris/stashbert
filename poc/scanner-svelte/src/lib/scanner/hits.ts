// Pure helpers for accepted scans.

export const DUPLICATE_WINDOW_MS = 2000;
export const RECENT_LIMIT = 10;

export interface Hit {
	/** Running number of the hit, also the counter value. */
	id: number;
	code: string;
	/** Time in ms since the epoch. */
	at: number;
}

/** The same code as the last hit within the window is ignored. */
export function isDuplicate(code: string, now: number, last: Hit | undefined): boolean {
	return last !== undefined && last.code === code && now - last.at < DUPLICATE_WINDOW_MS;
}

/** Newest first, at most RECENT_LIMIT entries. */
export function addRecent(recent: readonly Hit[], hit: Hit): Hit[] {
	return [hit, ...recent].slice(0, RECENT_LIMIT);
}
