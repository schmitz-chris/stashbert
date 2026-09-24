import { useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router";
import { NavigatedContext } from "../hooks/useHeadingFocus";
import { saveLastTab } from "../lib/lastTab";
import { BottomNav } from "./BottomNav";

/**
 * Layout of the views that show the bottom navigation bar. It remembers
 * the tab shown last, for the start of the app, and tells the headings of
 * the views whether the app navigated since the first load (docs/plan.md,
 * F24).
 */
export function TabLayout() {
  const { key, pathname } = useLocation();
  // Every navigation gives the location a new key.
  const [firstKey] = useState(key);

  useEffect(() => {
    saveLastTab(pathname);
  }, [pathname]);

  return (
    <div className="flex flex-1 flex-col">
      <main className="flex-1 px-4 py-6">
        <NavigatedContext value={key !== firstKey}>
          <Outlet />
        </NavigatedContext>
      </main>
      <BottomNav />
    </div>
  );
}
