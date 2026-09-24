import { useId, useState } from "react";
import { checkManualCode } from "../lib/manualCode";
import type { ScanMode } from "../lib/scan";
import { Dialog } from "./Dialog";

const buttonClass = "pressable min-h-11 rounded-lg px-4 font-medium";

// The submit button per mode of the scan view: its text and colour.
const submitButton: Record<ScanMode, { label: string; className: string }> = {
  add: { label: "Einlagern", className: "bg-accent text-white" },
  consume: { label: "Entnehmen", className: "bg-consume text-white" },
  mark: { label: "Vormerken", className: "bg-marked text-white" },
};

interface ManualCodeDialogProps {
  /** Whether the dialog is shown. */
  open: boolean;
  /** The current mode of the scan view; it names and colours the button. */
  mode: ScanMode;
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
export function ManualCodeDialog({ open, mode, onClose, onSubmit }: ManualCodeDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} title="Code eintippen">
      {/* Mounted only while open, so every opening starts empty. */}
      {open && <ManualCodeForm mode={mode} onCancel={onClose} onSubmit={onSubmit} />}
    </Dialog>
  );
}

function ManualCodeForm({
  mode,
  onCancel,
  onSubmit,
}: {
  mode: ScanMode;
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
      <label htmlFor={inputId} className="block text-sm font-medium text-ink-secondary">
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
        className="mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 py-2 font-mono text-base aria-[invalid=true]:border-danger"
      />
      <p id={errorId} role="status" className="mt-1 text-sm font-medium text-danger">
        {error}
      </p>
      <div className="mt-4 flex justify-end gap-3">
        <button
          type="button"
          onClick={onCancel}
          className={`border border-line-strong bg-surface text-ink-secondary ${buttonClass}`}
        >
          Abbrechen
        </button>
        <button type="submit" className={`${submitButton[mode].className} ${buttonClass}`}>
          {submitButton[mode].label}
        </button>
      </div>
    </form>
  );
}
