import type { ReactNode } from "react";
import type { FeedbackIcon } from "../lib/scan";

// The strokes of each symbol in a 24 × 24 box.
const strokes: Record<FeedbackIcon, ReactNode> = {
  check: <path d="M4.5 12.5 9.5 17.5 19.5 6.5" />,
  cart: (
    <>
      <path d="M2 3h2.5l2.6 12.2a1 1 0 0 0 1 .8h9.4a1 1 0 0 0 1-.76L21 7H5.6" />
      <circle cx="9" cy="20" r="1.5" />
      <circle cx="18" cy="20" r="1.5" />
    </>
  ),
  // An exclamation mark in a warning triangle.
  warning: (
    <>
      <path d="M12 3 2 20.5h20z" />
      <path d="M12 9.5v4.5M12 17.5v.01" />
    </>
  ),
  cross: <path d="M6 6l12 12M18 6 6 18" />,
};

/**
 * The symbol in front of the text of a scan message, drawn with the color
 * of the text. Screen readers skip it; the text says the same.
 */
export function FeedbackSymbol({ icon }: { icon: FeedbackIcon }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-8 shrink-0"
    >
      {strokes[icon]}
    </svg>
  );
}
