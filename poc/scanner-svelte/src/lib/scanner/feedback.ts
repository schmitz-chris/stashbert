// Audio feedback: Web Audio tone or <audio> element. Framework-independent.

const TONE_HZ = 880;
const TONE_SECONDS = 0.12;

type NavigatorWithAudioSession = Navigator & { audioSession?: { type: string } };

/** Lets the tone play even with the iOS silent switch on (where supported). */
export function setPlaybackAudioSession(): void {
	const audioSession = (navigator as NavigatorWithAudioSession).audioSession;
	if (audioSession) {
		audioSession.type = 'playback';
	}
}

export function playTone(context: AudioContext): void {
	const oscillator = context.createOscillator();
	const gain = context.createGain();
	const start = context.currentTime;
	const end = start + TONE_SECONDS;

	oscillator.frequency.value = TONE_HZ;
	gain.gain.setValueAtTime(0.5, start);
	gain.gain.linearRampToValueAtTime(0, end);
	oscillator.connect(gain).connect(context.destination);
	oscillator.start(start);
	oscillator.stop(end);
}

/**
 * Starts the element muted and pauses it right away. Called during a tap, so
 * iOS later allows play() without a tap.
 */
export function primeAudioElement(audio: HTMLAudioElement): void {
	audio.muted = true;
	const playing = audio.play();
	audio.pause();
	audio.currentTime = 0;
	audio.muted = false;
	// pause() rejects the pending play() with an AbortError; that is expected.
	playing.catch(() => {});
}

export function playAudioElement(audio: HTMLAudioElement): Promise<void> {
	audio.currentTime = 0;
	return audio.play();
}
