import { createContext, useContext, useEffect, useRef } from "react";

/**
 * Whether the app navigated since the tab layout appeared, that is since
 * the first load. TabLayout provides it; outside of it, it is false.
 */
export const NavigatedContext = createContext(false);

/**
 * Returns the ref for the h1 of a view. After a navigation within the app,
 * not on the first load, the heading takes the focus when it appears, so
 * a screen reader reads the new view (docs/plan.md, F24; HIG, VoiceOver).
 * A heading that stays, like that of a view whose tab is tapped again,
 * keeps its focus state.
 */
export function useHeadingFocus() {
  const ref = useRef<HTMLHeadingElement>(null);
  const navigated = useContext(NavigatedContext);
  // Only the value when the heading appears counts.
  const focusOnMount = useRef(navigated);
  useEffect(() => {
    if (focusOnMount.current) {
      // The view keeps its scroll position, e.g. after a back navigation.
      ref.current?.focus({ preventScroll: true });
    }
  }, []);
  return ref;
}
