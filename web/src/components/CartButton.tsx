import { CartIcon } from "./CartIcon";

/**
 * The round buttons in the rows of the stock and shopping lists
 * (docs/plan.md, F26): a face of 34 px (36 px at the default text size of
 * iOS), growing with the text up to 44 px, in a tap area of at least
 * 44 × 44 px. The tap area is a transparent border of 5 px around the face:
 * bg-clip-padding keeps the fill in the face, and the press state of
 * pressable lies inside the border too. Negative margins of the same width
 * keep the border out of the layout, so the face lines up with its
 * neighbors. The buttons lie above the link of the row (z-10). The caller
 * adds the colors of the face.
 */
export const roundButtonClass =
  "pressable relative z-10 -m-[5px] flex size-[max(44px,calc(min(2.125rem,44px)+10px))] shrink-0 items-center justify-center rounded-full border-[5px] border-transparent bg-clip-padding disabled:opacity-40";

/**
 * The cart button of a row: the cart on a gray face, or, checked (the
 * product is marked), the checked cart in the marked color on a light
 * orange face.
 */
export function CartButton({
  checked,
  label,
  pressed,
  disabled,
  onClick,
}: {
  checked: boolean;
  label: string;
  pressed?: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-pressed={pressed}
      aria-label={label}
      className={`${roundButtonClass} ${checked ? "bg-marked-soft text-marked" : "bg-fill text-ink-secondary"}`}
    >
      <CartIcon checked={checked} />
    </button>
  );
}
