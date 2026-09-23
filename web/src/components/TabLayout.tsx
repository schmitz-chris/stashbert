import { Outlet } from "react-router";
import { BottomNav } from "./BottomNav";

/** Layout of the views that show the bottom navigation bar. */
export function TabLayout() {
  return (
    <div className="flex flex-1 flex-col">
      <main className="flex-1 px-4 py-6">
        <Outlet />
      </main>
      <BottomNav />
    </div>
  );
}
