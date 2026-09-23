import { AudioFeedback, TONES } from "./scanner/feedback";

// The one AudioFeedback of the app. iOS plays Web Audio only after
// unlock() ran synchronously inside a tap, so the tap on the scan link
// and every tap in the scan view unlock it.
const feedback = new AudioFeedback();
let unlocked = false;

/** Unlocks the sound. Call it synchronously inside a click handler. */
export function unlockSound(): void {
  feedback.unlock();
  unlocked = true;
}

/** Reports whether unlockSound was called since the page was loaded. */
export function isSoundUnlocked(): boolean {
  return unlocked;
}

/** Plays the tone pattern name. Does nothing before unlockSound. */
export function playSound(name: keyof typeof TONES): void {
  feedback.play(TONES[name]);
}
