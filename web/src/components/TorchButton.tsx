import { useState } from "react";
import { hasTorch, setTorch } from "../lib/scanner/camera";

interface TorchButtonProps {
  /** The video track of the running camera, null if it does not run. */
  track: MediaStreamTrack | null;
}

/**
 * The round light button at the bottom left of the camera image, shown only
 * if the running camera has a light. Its size is in px, so it keeps it when
 * the text size of the system grows. The light belongs to one track: when
 * the camera stops (leaving the view, hidden page, another camera), the
 * track and its light end, and a new track starts with the button off.
 */
export function TorchButton({ track }: TorchButtonProps) {
  // The track whose light is on, null if no light is on.
  const [litTrack, setLitTrack] = useState<MediaStreamTrack | null>(null);

  if (track === null || !hasTorch(track)) {
    return null;
  }
  const on = litTrack === track;

  function toggle(current: MediaStreamTrack) {
    const next = !on;
    setTorch(current, next).then(
      () => setLitTrack(next ? current : null),
      (error: unknown) => console.warn("Licht:", error),
    );
  }

  return (
    <button
      type="button"
      aria-label="Licht"
      aria-pressed={on}
      onClick={() => toggle(track)}
      className={`pressable absolute bottom-[8px] left-[8px] flex size-[48px] items-center justify-center rounded-full shadow-md ${on ? "bg-surface text-ink" : "bg-black/60 text-white ring-1 ring-white/40"}`}
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        fill={on ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth={2}
        strokeLinejoin="round"
        className="size-[24px]"
      >
        <path d="M13 2 4 14h7l-1 8 9-12h-7z" />
      </svg>
    </button>
  );
}
