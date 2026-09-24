/**
 * The cross of the buttons that close a card or a message, drawn with the
 * color of the text. The button names the action (aria-label).
 */
export function CloseIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      className="size-5"
    >
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  );
}
