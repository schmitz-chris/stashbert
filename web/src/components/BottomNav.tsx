import type { ReactNode } from "react";
import { NavLink, useLocation, useMatch } from "react-router";
import { productBack } from "../lib/productOrigin";
import { unlockSound } from "../lib/sound";

function sideLinkClass(marked: boolean) {
  const state = marked
    ? "bg-accent-soft font-semibold text-accent"
    : "font-medium text-ink-tertiary";
  return `pressable flex min-h-[48px] min-w-[80px] flex-col items-center justify-center gap-[2px] rounded-[8px] px-[12px] py-[4px] text-[12px] leading-[16px] ${state}`;
}

// The marked Scan button gets a ring in the accent color, set apart from
// the button by a gap in the color of the bar.
function scanLinkClass(marked: boolean) {
  const state = marked
    ? "bg-accent-strong ring-[3px] ring-accent ring-offset-[3px] ring-offset-surface"
    : "bg-accent";
  return `pressable flex size-[64px] -translate-y-[12px] flex-col items-center justify-center gap-[2px] rounded-full text-[12px] leading-[16px] font-semibold text-white shadow-md ${state}`;
}

/**
 * The bottom navigation bar of the views Vorrat, Scan and Einkauf, of the
 * product page and of the settings. On the product page the tab of the
 * view it was opened from is marked, the view its back link names
 * (docs/plan.md, F22); on the settings, which open from Vorrat and go back
 * there, Vorrat is marked (F32). The marked tab shows its symbol filled,
 * the others as outlines (HIG, Tab bars). The bar is slightly translucent
 * and blurs the content below it, close to the material of iOS
 * (docs/plan.md, F24). Its sizes are in px, so it keeps them when the text
 * size of the system grows, like the tab bars of iOS (HIG, Typography: tab
 * titles do not grow), and never wraps or reaches past the edge of the
 * screen (ADR-0016).
 */
export function BottomNav() {
  const { state } = useLocation();
  const onProduct = useMatch("/produkt/:id") !== null;
  const onSettings = useMatch("/einstellungen") !== null;
  // The path of the tab that is marked although its route is not active.
  let origin: string | null = null;
  if (onProduct) {
    origin = productBack(state).path;
  } else if (onSettings) {
    origin = "/vorrat";
  }

  return (
    <nav
      aria-label="Hauptnavigation"
      className="sticky bottom-0 z-20 border-t border-line bg-surface/85 pb-[env(safe-area-inset-bottom)] backdrop-blur-xl"
    >
      <ul className="mx-auto grid h-[64px] max-w-[448px] grid-cols-3 items-center">
        <li className="flex justify-center">
          <TabLink to="/vorrat" origin={origin} linkClass={sideLinkClass} Icon={BoxIcon}>
            Vorrat
          </TabLink>
        </li>
        <li className="flex justify-center">
          {/* The tap unlocks the sound of the scan view (iOS). */}
          <TabLink
            to="/scan"
            origin={origin}
            linkClass={scanLinkClass}
            Icon={BarcodeIcon}
            onClick={() => unlockSound()}
          >
            Scan
          </TabLink>
        </li>
        <li className="flex justify-center">
          <TabLink to="/einkauf" origin={origin} linkClass={sideLinkClass} Icon={CartIcon}>
            Einkauf
          </TabLink>
        </li>
      </ul>
    </nav>
  );
}

// A tab of the bar. It is marked while its route is active or when it is
// origin; its class and its symbol follow the mark.
function TabLink({
  to,
  origin,
  linkClass,
  Icon,
  onClick,
  children,
}: {
  to: string;
  origin: string | null;
  linkClass: (marked: boolean) => string;
  Icon: (props: { filled: boolean }) => ReactNode;
  onClick?: () => void;
  children: ReactNode;
}) {
  return (
    <NavLink
      to={to}
      onClick={onClick}
      className={({ isActive }) => linkClass(isActive || origin === to)}
    >
      {({ isActive }) => (
        <>
          <Icon filled={isActive || origin === to} />
          {children}
        </>
      )}
    </NavLink>
  );
}

// The symbols of the bar, drawn with the color of the text.
function NavIcon({ className, children }: { className: string; children: ReactNode }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      {children}
    </svg>
  );
}

// A box for Vorrat. Filled, its edges are drawn in the color of the marked
// tab behind it.
function BoxIcon({ filled }: { filled: boolean }) {
  return (
    <NavIcon className="size-[24px]">
      <path d="M12 3 3 7.5v9L12 21l9-4.5v-9z" fill={filled ? "currentColor" : "none"} />
      <path
        d="M3 7.5 12 12l9-4.5M12 12v9"
        stroke={filled ? "var(--color-accent-soft)" : "currentColor"}
        strokeWidth={filled ? 1.5 : 2}
      />
    </NavIcon>
  );
}

// A barcode in a viewfinder for Scan. Filled, the viewfinder is a white
// face with the bars in the color of the marked button.
function BarcodeIcon({ filled }: { filled: boolean }) {
  const bars = filled ? "var(--color-accent-strong)" : "currentColor";
  return (
    <NavIcon className="size-[28px]">
      {filled ? (
        <rect x="3" y="3" width="18" height="18" rx="2" fill="currentColor" />
      ) : (
        <path d="M3 8V5a2 2 0 0 1 2-2h3M16 3h3a2 2 0 0 1 2 2v3M21 16v3a2 2 0 0 1-2 2h-3M8 21H5a2 2 0 0 1-2-2v-3" />
      )}
      <path d="M7 7v10M12 7v10M17 7v10" stroke={bars} strokeLinecap="butt" />
      <path d="M9.5 7v10M14.5 7v10" stroke={bars} strokeWidth={1} strokeLinecap="butt" />
    </NavIcon>
  );
}

// A shopping cart for Einkauf. Filled, the basket is a solid face.
function CartIcon({ filled }: { filled: boolean }) {
  return (
    <NavIcon className="size-[24px]">
      {filled ? (
        <>
          <path d="M2 3h2.5l.85 4" />
          <path d="M5.35 7H21l-2.5 8.24a1 1 0 0 1-1 .76H8.1a1 1 0 0 1-1-.8z" fill="currentColor" />
        </>
      ) : (
        <path d="M2 3h2.5l2.6 12.2a1 1 0 0 0 1 .8h9.4a1 1 0 0 0 1-.76L21 7H5.6" />
      )}
      <circle cx="9" cy="20" r="1.5" fill={filled ? "currentColor" : "none"} />
      <circle cx="18" cy="20" r="1.5" fill={filled ? "currentColor" : "none"} />
    </NavIcon>
  );
}
