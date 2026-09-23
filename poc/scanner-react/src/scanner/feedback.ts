// Hit feedback: Web Audio tone, or beep.wav through an <audio> element.

const TONE_HZ = 880;
const TONE_MS = 120;
const RAMP_S = 0.005;

// navigator.audioSession exists in Safari 16.4+, not in the TypeScript DOM typings.
type AudioSessionNavigator = Navigator & { audioSession?: { type: string } };

export class AudioFeedback {
  private context: AudioContext | null = null;

  // Must be called synchronously inside a tap handler.
  unlock(): void {
    const { audioSession } = navigator as AudioSessionNavigator;
    if (audioSession) {
      audioSession.type = "playback";
    }
    this.context ??= new AudioContext();
    if (this.context.state !== "running") {
      void this.context.resume();
    }
  }

  beep(): void {
    const context = this.context;
    if (!context) {
      return;
    }
    const start = context.currentTime;
    const end = start + TONE_MS / 1000;
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    oscillator.frequency.value = TONE_HZ;
    gain.gain.setValueAtTime(0, start);
    gain.gain.linearRampToValueAtTime(0.5, start + RAMP_S);
    gain.gain.setValueAtTime(0.5, end - RAMP_S);
    gain.gain.linearRampToValueAtTime(0, end);
    oscillator.connect(gain).connect(context.destination);
    oscillator.start(start);
    oscillator.stop(end);
  }
}

// iOS only allows play() without a tap once the element has been played
// during a tap. Called from tap handlers, it plays the element muted and
// stops it right away.
export function primeAudioElement(audio: HTMLAudioElement): void {
  audio.muted = true;
  audio
    .play()
    .then(() => audio.pause())
    .catch(() => undefined)
    .finally(() => {
      audio.currentTime = 0;
      audio.muted = false;
    });
}

export function playAudioElement(audio: HTMLAudioElement): Promise<void> {
  audio.currentTime = 0;
  return audio.play();
}
