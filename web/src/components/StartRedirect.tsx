import { Navigate } from "react-router";
import { loadStartPath } from "../lib/lastTab";

/**
 * Leads the start of the app (/) to the tab shown last, or to Vorrat
 * (docs/plan.md, F24). replace keeps / out of the history.
 */
export function StartRedirect() {
  return <Navigate to={loadStartPath()} replace />;
}
