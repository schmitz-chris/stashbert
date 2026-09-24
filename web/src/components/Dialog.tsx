import { useEffect, useId, useRef, type ReactNode, type SyntheticEvent } from "react";

interface DialogProps {
  /** Whether the dialog is shown. */
  open: boolean;
  /** Called when the dialog closed itself, for example by Escape. */
  onClose: () => void;
  title: string;
  children: ReactNode;
}

// Shows the <dialog> of the returned ref with showModal() while open is
// true. A later one lies above an earlier one, e.g. a confirmation above a
// sheet.
function useModal(open: boolean) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    if (!open || dialog === null) {
      return;
    }
    dialog.showModal();
    return () => dialog.close();
  }, [open]);
  return ref;
}

// The close event comes after close(). If the dialog was shown again in
// between (effects run twice in StrictMode), it is not closed.
function closeHandler(onClose: () => void) {
  return (event: SyntheticEvent<HTMLDialogElement>) => {
    if (!event.currentTarget.open) {
      onClose();
    }
  };
}

/**
 * A modal dialog on the native <dialog> element, shown with showModal().
 * The parent owns open; when the dialog closes itself (Escape), it calls
 * onClose, so the parent can set open to false.
 */
export function Dialog({ open, onClose, title, children }: DialogProps) {
  const ref = useModal(open);
  const titleId = useId();

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onClose={closeHandler(onClose)}
      className="m-auto w-[calc(100%-2rem)] max-w-sm rounded-xl bg-surface p-5 text-ink shadow-xl backdrop:bg-ink/40"
    >
      <h2 id={titleId} className="text-lg font-semibold">
        {title}
      </h2>
      {children}
    </dialog>
  );
}

/**
 * A sheet on the native <dialog> element (HIG, Sheets): at the bottom edge
 * over the full width, with rounded top corners and a grabber that is only
 * drawn (no swiping). "Abbrechen" on the leading edge of its top bar
 * closes it like Escape and calls onClose; the title is in the middle.
 */
export function Sheet({ open, onClose, title, children }: DialogProps) {
  const ref = useModal(open);
  const titleId = useId();

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onClose={closeHandler(onClose)}
      className="mx-0 mt-auto mb-0 max-h-[calc(100dvh-env(safe-area-inset-top)-1.5rem)] w-full max-w-none rounded-t-2xl bg-surface pt-2 pr-[max(1rem,env(safe-area-inset-right))] pb-[calc(1rem+env(safe-area-inset-bottom))] pl-[max(1rem,env(safe-area-inset-left))] text-ink shadow-xl backdrop:bg-ink/40"
    >
      {/* The grabber, a shade of ink like the press state. */}
      <div aria-hidden="true" className="mx-auto h-[5px] w-9 rounded-full bg-ink/20" />
      <div className="mt-1 grid grid-cols-[1fr_auto_1fr] items-center gap-2">
        <button
          type="button"
          onClick={() => ref.current?.close()}
          className="pressable -ml-2 min-h-11 justify-self-start rounded-lg px-2 font-medium text-accent"
        >
          Abbrechen
        </button>
        <h2 id={titleId} className="text-center text-lg font-semibold">
          {title}
        </h2>
      </div>
      {children}
    </dialog>
  );
}

interface ConfirmActionsProps {
  /** The title of the button that performs the destructive action. */
  label: string;
  /** Whether the request of the action runs; the button waits meanwhile. */
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

const confirmButtonClass =
  "pressable min-h-11 flex-1 rounded-lg border border-line-strong bg-surface px-4 font-medium whitespace-nowrap disabled:opacity-40";

/**
 * The buttons of a confirmation (HIG, Alerts and Buttons): "Abbrechen" and
 * the destructive action with the same weight, the action only in red
 * text and never filled. In a row "Abbrechen" is on the leading side; when
 * both do not fit (large text) they stack, the action on top.
 */
export function ConfirmActions({ label, pending, onCancel, onConfirm }: ConfirmActionsProps) {
  return (
    // wrap-reverse puts the second line (the action) above the first.
    <div className="mt-4 flex flex-wrap-reverse gap-3">
      <button type="button" onClick={onCancel} className={`${confirmButtonClass} text-ink`}>
        Abbrechen
      </button>
      <button
        type="button"
        disabled={pending}
        onClick={onConfirm}
        className={`${confirmButtonClass} text-danger`}
      >
        {label}
      </button>
    </div>
  );
}
