import { useId, useState } from "react";
import { checkManualCode } from "../lib/manualCode";
import { Dialog } from "./Dialog";

const buttonClass = "min-h-11 rounded-lg px-4 font-medium";

interface ManualCodeDialogProps {
  /** Whether the dialog is shown. */
  open: boolean;
  /** Called when the user closes the dialog without booking. */
  onClose: () => void;
  /** Called with the canonical code of a valid input. */
  onSubmit: (code: string) => void;
}

/**
 * The dialog of the scan view to type in a code by hand (docs/plan.md,
 * F11). A valid code is passed to onSubmit; for an invalid one the dialog
 * shows "Ungültiger Barcode" and stays open.
 */
export function ManualCodeDialog({ open, onClose, onSubmit }: ManualCodeDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} title="Code eintippen">
      {/* Mounted only while open, so every opening starts empty. */}
      {open && <ManualCodeForm onCancel={onClose} onSubmit={onSubmit} />}
    </Dialog>
  );
}

function ManualCodeForm({
  onCancel,
  onSubmit,
}: {
  onCancel: () => void;
  onSubmit: (code: string) => void;
}) {
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const inputId = useId();
  const errorId = useId();

  function submit() {
    const checked = checkManualCode(input);
    if (!checked.ok) {
      setError(checked.error);
      return;
    }
    onSubmit(checked.code);
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
      className="mt-3"
    >
      <label htmlFor={inputId} className="block text-sm font-medium text-stone-700">
        Barcode
      </label>
      <input
        id={inputId}
        value={input}
        onChange={(event) => {
          setInput(event.target.value);
          setError(null);
        }}
        inputMode="numeric"
        autoComplete="off"
        aria-invalid={error !== null}
        aria-describedby={error !== null ? errorId : undefined}
        className="mt-1 min-h-11 w-full rounded-lg border border-stone-300 bg-white px-3 py-2 font-mono text-base aria-[invalid=true]:border-red-600"
      />
      <p id={errorId} role="status" className="mt-1 text-sm font-medium text-red-700">
        {error}
      </p>
      <div className="mt-4 flex justify-end gap-3">
        <button
          type="button"
          onClick={onCancel}
          className={`border border-stone-300 bg-white text-stone-700 ${buttonClass}`}
        >
          Abbrechen
        </button>
        <button type="submit" className={`bg-emerald-600 text-white ${buttonClass}`}>
          Buchen
        </button>
      </div>
    </form>
  );
}
