import { useId, useState } from "react";
import { crateChoices, parseCrateSize } from "../lib/resultCard";
import { Dialog } from "./Dialog";

const buttonClass = "pressable min-h-11 rounded-lg px-4 font-medium";

interface CrateSizeDialogProps {
  /** Whether the dialog is shown. */
  open: boolean;
  /** Called when the user closes the dialog without a choice. */
  onClose: () => void;
  /** Called with the chosen crate size, from 2 to 100. */
  onChoose: (size: number) => void;
}

/**
 * The choice of "War ein Kasten" on the result card (docs/plan.md, F28):
 * a button for each size of crateChoices, which chooses at once, or
 * another number from 2 to 100, typed in. An invalid number shows
 * "Ungültige Kastengröße" and the dialog stays open.
 */
export function CrateSizeDialog({ open, onClose, onChoose }: CrateSizeDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} title="War ein Kasten">
      {/* Mounted only while open, so every opening starts empty. */}
      {open && <CrateSizeForm onCancel={onClose} onChoose={onChoose} />}
    </Dialog>
  );
}

function CrateSizeForm({
  onCancel,
  onChoose,
}: {
  onCancel: () => void;
  onChoose: (size: number) => void;
}) {
  const [input, setInput] = useState("");
  const [invalid, setInvalid] = useState(false);
  const labelId = useId();
  const inputId = useId();
  const errorId = useId();

  function submit() {
    const size = parseCrateSize(input);
    if (size === null) {
      setInvalid(true);
      return;
    }
    onChoose(size);
  }

  return (
    <>
      <p id={labelId} className="mt-1 text-ink-secondary">
        Flaschen pro Kasten
      </p>
      {/* Four in a row; fewer when the text is large. */}
      <div
        role="group"
        aria-labelledby={labelId}
        className="mt-3 grid grid-cols-[repeat(auto-fit,minmax(3rem,1fr))] gap-2"
      >
        {crateChoices.map((size) => (
          <button
            key={size}
            type="button"
            onClick={() => onChoose(size)}
            className="pressable min-h-14 rounded-lg bg-fill text-xl font-semibold text-ink tabular-nums"
          >
            {size}
          </button>
        ))}
      </div>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
        className="mt-4"
      >
        <label htmlFor={inputId} className="block text-sm font-medium text-ink-secondary">
          Andere Zahl (2 bis 100)
        </label>
        <input
          id={inputId}
          value={input}
          onChange={(event) => {
            setInput(event.target.value);
            setInvalid(false);
          }}
          inputMode="numeric"
          autoComplete="off"
          aria-invalid={invalid}
          aria-describedby={invalid ? errorId : undefined}
          className="mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 py-2 text-base tabular-nums aria-[invalid=true]:border-danger"
        />
        <p id={errorId} role="status" className="mt-1 text-sm font-medium text-danger">
          {invalid ? "Ungültige Kastengröße" : null}
        </p>
        <div className="mt-4 flex flex-wrap justify-end gap-3">
          <button
            type="button"
            onClick={onCancel}
            className={`border border-line-strong bg-surface text-ink-secondary ${buttonClass}`}
          >
            Abbrechen
          </button>
          <button type="submit" className={`bg-accent text-white ${buttonClass}`}>
            Einlagern
          </button>
        </div>
      </form>
    </>
  );
}
