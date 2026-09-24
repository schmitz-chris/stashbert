/**
 * The shopping cart of the cart buttons in the stock and shopping lists
 * and of the status "vorgemerkt", drawn with the color of the text like
 * the symbols of the navigation bar; checked, with a tick in the basket.
 * By default it has the size for a round button (see CartButton), 20 px,
 * growing with the text size up to 24 px.
 */
export function CartIcon({
  checked,
  className = "size-[min(1.25rem,24px)]",
}: {
  checked: boolean;
  className?: string;
}) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`shrink-0 ${className}`}
    >
      <path d="M2 3h2.5l2.6 12.2a1 1 0 0 0 1 .8h9.4a1 1 0 0 0 1-.76L21 7H5.6" />
      <circle cx="9" cy="20" r="1.5" />
      <circle cx="18" cy="20" r="1.5" />
      {checked && <path d="m10 11.5 2 2 4-4" />}
    </svg>
  );
}
