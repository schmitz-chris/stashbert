import { useRegisterSW } from "virtual:pwa-register/react";

/**
 * Registers the service worker and shows a banner once a new version is
 * waiting. The new version only takes over after a tap, so it never reloads
 * the page in the middle of a scan. It sits at the top of every view and
 * stays visible while scrolling; the safe area comes from #root.
 */
export function UpdateBanner() {
  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW();

  if (!needRefresh) {
    return null;
  }
  return (
    <div role="status" className="sticky top-[env(safe-area-inset-top)] z-20">
      <button
        type="button"
        onClick={() => void updateServiceWorker(true)}
        className="pressable min-h-12 w-full bg-accent px-4 py-2 font-medium text-white"
      >
        Neue Version verfügbar, tippen zum Aktualisieren
      </button>
    </div>
  );
}
