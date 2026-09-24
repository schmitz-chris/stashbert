import type { ReactNode } from "react";
import { useHeadingFocus } from "../hooks/useHeadingFocus";

/**
 * The h1 of a view. After a navigation it takes the focus (see
 * useHeadingFocus). It is focusable only by script (tabIndex -1) and shows
 * a focus ring only when the keyboard was used last (focus-visible), not
 * after a tap or a click. The ring lies inside the box, so the sticky bar
 * below the heading of Vorrat does not cover it; the box reaches 8 px
 * past the text on both sides (-mx-2 px-2), so the text stays in place and
 * keeps its distance from the ring.
 */
export function PageHeading({ className, children }: { className: string; children: ReactNode }) {
  const ref = useHeadingFocus();
  return (
    <h1
      ref={ref}
      tabIndex={-1}
      className={`${className} -mx-2 rounded-md px-2 focus:outline-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent focus-visible:outline-solid`}
    >
      {children}
    </h1>
  );
}
