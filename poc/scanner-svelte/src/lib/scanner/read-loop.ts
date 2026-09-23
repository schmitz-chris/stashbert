// Calls an async attempt repeatedly: at most once per interval and never twice at the same time.

export const MIN_INTERVAL_MS = 100; // at most 10 attempts per second

export class ReadLoop {
	#attempt: () => Promise<void>;
	#generation = 0;
	#inFlight: Promise<void> = Promise.resolve();
	#timer: ReturnType<typeof setTimeout> | undefined;

	constructor(attempt: () => Promise<void>) {
		this.#attempt = attempt;
	}

	start(): void {
		this.stop();
		const generation = this.#generation;

		const tick = async () => {
			// A restart must not overlap with an attempt of the previous run.
			await this.#inFlight;
			if (generation !== this.#generation) return;

			const started = performance.now();
			// The attempt reports its own errors; a rejection must not end the loop.
			this.#inFlight = this.#attempt().catch((error: unknown) => console.error(error));
			await this.#inFlight;
			if (generation !== this.#generation) return;

			const wait = Math.max(0, MIN_INTERVAL_MS - (performance.now() - started));
			this.#timer = setTimeout(tick, wait);
		};

		void tick();
	}

	stop(): void {
		this.#generation += 1;
		clearTimeout(this.#timer);
	}
}
