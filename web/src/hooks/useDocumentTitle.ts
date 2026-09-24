import { useEffect } from "react";

/**
 * Sets the title of the document while the view is shown (docs/plan.md,
 * F24) and restores the one before when the view goes.
 */
export function useDocumentTitle(title: string) {
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}
