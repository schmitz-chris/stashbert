/**
 * The tab used last (docs/plan.md, F24): the app remembers which of the
 * views Vorrat, Scan and Einkauf was shown last, and its start (/) leads
 * there again (HIG, Launching: restore the previous state).
 */

/** The paths of the tabs of the navigation bar. */
export type TabPath = "/vorrat" | "/scan" | "/einkauf";

const tabKey = "stashbert.lastTab";
const firstTab: TabPath = "/vorrat";

/** Reports whether path is the path of a tab. */
export function isTabPath(path: unknown): path is TabPath {
  return path === "/vorrat" || path === "/scan" || path === "/einkauf";
}

/**
 * Returns the path the start of the app leads to for the saved value
 * saved: the path of the tab it names, or /vorrat if nothing or an unknown
 * value is saved.
 */
export function startPath(saved: string | null): TabPath {
  return isTabPath(saved) ? saved : firstTab;
}

/** Returns the path the start of the app leads to (see startPath). */
export function loadStartPath(): TabPath {
  try {
    return startPath(localStorage.getItem(tabKey));
  } catch {
    return firstTab;
  }
}

/**
 * Saves pathname as the tab used last if it is the path of a tab. Any
 * other path, like that of a product page, keeps the saved tab.
 */
export function saveLastTab(pathname: string): void {
  if (!isTabPath(pathname)) {
    return;
  }
  try {
    localStorage.setItem(tabKey, pathname);
  } catch {
    // Storage unavailable: the app starts with Vorrat.
  }
}
