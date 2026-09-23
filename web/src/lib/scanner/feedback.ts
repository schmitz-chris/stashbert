// Scan feedback: tone patterns over Web Audio.

const RAMP_S = 0.005;
const PAUSE_MS = 60;

export interface Tone {
  hz: number;
  ms: number;
}

// Tone patterns per outcome (docs/plan.md, F07).
export const TONES = {
  add: [{ hz: 880, ms: 100 }],
  consume: [{ hz: 660, ms: 100 }],
  warn: [
    { hz: 440, ms: 80 },
    { hz: 440, ms: 80 },
  ],
  error: [{ hz: 220, ms: 300 }],
} satisfies Record<string, Tone[]>;

// navigator.audioSession exists in Safari 16.4+, not in the TypeScript DOM typings.
type AudioSessionNavigator = Navigator & { audioSession?: { type: string } };

export class AudioFeedback {
  private context: AudioContext | null = null;

  // Must be called synchronously inside a tap handler (the start tap).
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

  // Plays the tones one after the other with a short pause in between.
  // Does nothing before unlock().
  play(tones: readonly Tone[]): void {
    const context = this.context;
    if (!context) {
      return;
    }
    let start = context.currentTime;
    for (const tone of tones) {
      const end = start + tone.ms / 1000;
      const oscillator = context.createOscillator();
      const gain = context.createGain();
      oscillator.frequency.value = tone.hz;
      gain.gain.setValueAtTime(0, start);
      gain.gain.linearRampToValueAtTime(0.5, start + RAMP_S);
      gain.gain.setValueAtTime(0.5, end - RAMP_S);
      gain.gain.linearRampToValueAtTime(0, end);
      oscillator.connect(gain).connect(context.destination);
      oscillator.start(start);
      oscillator.stop(end);
      start = end + PAUSE_MS / 1000;
    }
  }
}
