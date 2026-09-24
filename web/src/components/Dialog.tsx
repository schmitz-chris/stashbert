import { useEffect, useId, useRef, type ReactNode } from "react";

interface DialogProps {
  /** Whether the dialog is shown. */
  open: boolean;
  /** Called when the dialog closed itself, for example by Escape. */
  onClose: () => void;
  title: string;
  children: ReactNode;
}

/**
 * A modal dialog on the native <dialog> element, shown with showModal().
 * The parent owns open; when the dialog closes itself (Escape), it calls
 * onClose, so the parent can set open to false.
 */
export function Dialog({ open, onClose, title, children }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (!open || dialog === null) {
      return;
    }
    dialog.showModal();
    return () => dialog.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      // The close event comes after close(). If the dialog was shown again
      // in between (effects run twice in StrictMode), it is not closed.
      onClose={(event) => {
        if (!event.currentTarget.open) {
          onClose();
        }
      }}
      className="m-auto w-[calc(100%-2rem)] max-w-sm rounded-xl bg-surface p-5 text-ink shadow-xl backdrop:bg-ink/40"
    >
      <h2 id={titleId} className="text-lg font-semibold">
        {title}
      </h2>
      {children}
    </dialog>
  );
}
