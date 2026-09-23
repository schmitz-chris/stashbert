import { NavLink, type NavLinkRenderProps } from "react-router";

function sideLinkClass({ isActive }: NavLinkRenderProps) {
  const state = isActive
    ? "bg-emerald-50 font-semibold text-emerald-700"
    : "text-stone-600";
  return `flex min-h-12 min-w-20 items-center justify-center rounded-lg px-3 ${state}`;
}

function scanLinkClass({ isActive }: NavLinkRenderProps) {
  const state = isActive
    ? "bg-emerald-700 ring-4 ring-emerald-200"
    : "bg-emerald-600";
  return `flex size-16 -translate-y-3 items-center justify-center rounded-full text-lg font-semibold text-white shadow-md ${state}`;
}

/** The bottom navigation bar of the views Vorrat, Scan and Einkauf. */
export function BottomNav() {
  return (
    <nav
      aria-label="Hauptnavigation"
      className="sticky bottom-0 border-t border-stone-200 bg-white pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="mx-auto grid h-16 max-w-md grid-cols-3 items-center">
        <li className="flex justify-center">
          <NavLink to="/vorrat" className={sideLinkClass}>
            Vorrat
          </NavLink>
        </li>
        <li className="flex justify-center">
          <NavLink to="/scan" className={scanLinkClass}>
            Scan
          </NavLink>
        </li>
        <li className="flex justify-center">
          <NavLink to="/einkauf" className={sideLinkClass}>
            Einkauf
          </NavLink>
        </li>
      </ul>
    </nav>
  );
}
