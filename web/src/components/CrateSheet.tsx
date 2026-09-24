import type { CrateQuestion } from "../lib/scan";
import { Sheet } from "./Dialog";

const answerClass =
  "pressable min-h-14 w-full rounded-xl bg-accent px-4 py-2 text-xl font-semibold break-words text-white";

interface CrateSheetProps {
  /** The open question; the sheet is shown while it is not null. */
  question: CrateQuestion | null;
  /** Called by "Abbrechen" and Escape; nothing is booked. */
  onClose: () => void;
  /** Called with the quantity to book: 1 or the crate size. */
  onAnswer: (quantity: number) => void;
}

/**
 * The question "Flasche oder Kasten" before a code of a product with a
 * crate size is booked in mode add (docs/plan.md, F28): a sheet with the
 * name of the product and two large buttons, "Flasche" and "Kasten (N
 * Flaschen)". "Abbrechen" books nothing.
 */
export function CrateSheet({ question, onClose, onAnswer }: CrateSheetProps) {
  return (
    <Sheet open={question !== null} onClose={onClose} title="Flasche oder Kasten?">
      {question !== null && (
        <>
          <p className="mt-1 text-center break-words hyphens-auto text-ink-secondary">
            {question.name}
          </p>
          <div className="mt-4 flex flex-col gap-3">
            <button type="button" onClick={() => onAnswer(1)} className={answerClass}>
              Flasche
            </button>
            <button
              type="button"
              onClick={() => onAnswer(question.crateSize)}
              className={answerClass}
            >
              Kasten ({question.crateSize} Flaschen)
            </button>
          </div>
        </>
      )}
    </Sheet>
  );
}
