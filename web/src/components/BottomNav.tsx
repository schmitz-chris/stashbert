import type { ReactNode } from "react";
import { NavLink, type NavLinkRenderProps } from "react-router";
import { unlockSound } from "../lib/sound";

function sideLinkClass({ isActive }: NavLinkRenderProps) {
  const state = isActive
    ? "bg-accent-soft font-semibold text-accent"
    : "font-medium text-ink-tertiary";
  return `pressable flex min-h-12 min-w-20 flex-col items-center justify-center gap-0.5 rounded-lg px-3 py-1 text-xs ${state}`;
}

function scanLinkClass({ isActive }: NavLinkRenderProps) {
  const state = isActive
    ? "bg-accent-strong ring-4 ring-accent/30"
    : "bg-accent";
  return `pressable flex size-16 -translate-y-3 flex-col items-center justify-center gap-0.5 rounded-full text-xs font-semibold text-white shadow-md ${state}`;
}

/** The bottom navigation bar of the views Vorrat, Scan and Einkauf. */
export function BottomNav() {
  return (
    <nav
      aria-label="Hauptnavigation"
      className="sticky bottom-0 z-20 border-t border-line bg-surface pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="mx-auto grid h-16 max-w-md grid-cols-3 items-center">
        <li className="flex justify-center">
          <NavLink to="/vorrat" className={sideLinkClass}>
            <BoxIcon />
            Vorrat
          </NavLink>
        </li>
        <li className="flex justify-center">
          {/* The tap unlocks the sound of the scan view (iOS). */}
          <NavLink to="/scan" className={scanLinkClass} onClick={() => unlockSound()}>
            <BarcodeIcon />
            Scan
          </NavLink>
        </li>
        <li className="flex justify-center">
          <NavLink to="/einkauf" className={sideLinkClass}>
            <CartIcon />
            Einkauf
          </NavLink>
        </li>
      </ul>
    </nav>
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

// A box for Vorrat.
function BoxIcon() {
  return (
    <NavIcon className="size-6">
      <path d="M12 3 3 7.5v9L12 21l9-4.5v-9z" />
      <path d="M3 7.5 12 12l9-4.5M12 12v9" />
    </NavIcon>
  );
}

// A barcode in a viewfinder for Scan.
function BarcodeIcon() {
  return (
    <NavIcon className="size-7">
      <path d="M3 8V5a2 2 0 0 1 2-2h3M16 3h3a2 2 0 0 1 2 2v3M21 16v3a2 2 0 0 1-2 2h-3M8 21H5a2 2 0 0 1-2-2v-3" />
      <path d="M7 7v10M12 7v10M17 7v10" strokeLinecap="butt" />
      <path d="M9.5 7v10M14.5 7v10" strokeWidth={1} strokeLinecap="butt" />
    </NavIcon>
  );
}

// A shopping cart for Einkauf.
function CartIcon() {
  return (
    <NavIcon className="size-6">
      <path d="M2 3h2.5l2.6 12.2a1 1 0 0 0 1 .8h9.4a1 1 0 0 0 1-.76L21 7H5.6" />
      <circle cx="9" cy="20" r="1.5" />
      <circle cx="18" cy="20" r="1.5" />
    </NavIcon>
  );
}
