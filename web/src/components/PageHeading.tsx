import type { ReactNode } from "react";
import { useHeadingFocus } from "../hooks/useHeadingFocus";

/**
 * The h1 of a view. After a navigation it takes the focus (see
 * useHeadingFocus), so screen readers announce the new view. It is
 * focusable only by script (tabIndex -1) and never shows a focus ring: it
 * is not a control, and iOS Safari showed the ring after a tap (user
 * feedback of 2026-09-24).
 */
export function PageHeading({ className, children }: { className: string; children: ReactNode }) {
  const ref = useHeadingFocus();
  return (
    <h1
      ref={ref}
      tabIndex={-1}
      className={`${className} outline-none`}
    >
      {children}
    </h1>
  );
}
