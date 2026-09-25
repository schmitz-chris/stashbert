import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState, type Ref } from "react";
import { Link } from "react-router";
import { productUpdateMutation } from "../lib/api/queries";
import { productLinkState } from "../lib/productOrigin";
import type { Product } from "../lib/products";
import {
  asksForName,
  cardView,
  markCardView,
  maxNameLength,
  nameToSave,
  openGtinDbUrl,
  placeholderCode,
  targetChoices,
  type CardBooking,
  type CardMark,
} from "../lib/resultCard";
import { CloseIcon } from "./CloseIcon";

// How long the message of a saved target stays visible. A failure stays
// until the next choice (ADR-0016).
const noticeDuration = 2000;

const actionClass =
  "pressable inline-flex min-h-11 items-center rounded-lg border border-line-strong bg-surface px-3 font-medium hyphens-auto text-ink";

const undoClass =
  "pressable min-h-11 shrink-0 rounded-lg bg-fill px-3 font-medium text-ink disabled:opacity-40";

interface ResultCardProps {
  booking: CardBooking;
  /** Locks both buttons, while an action of the card runs. */
  disabled: boolean;
  /**
   * The name field of a placeholder, so the scan view can ignore codes
   * while it has the focus (docs/plan.md, F31).
   */
  nameInputRef: Ref<HTMLInputElement>;
  onPlusOne: () => void;
  onUndo: () => void;
  /** Called with the product from the response after its target or its name changed. */
  onProductChange: (product: Product) => void;
  /** Called by "Stattdessen zu vorhandenem Produkt". */
  onMerge: () => void;
  /** Called by "Karte schließen". */
  onClose: () => void;
}

/**
 * The result card of the scan view: the product of the booking, its
 * stock before and after, and the buttons [+1] and [Rückgängig]. For a
 * booking that created its product it also offers the target chips, a
 * link to the product page and the merge into another product. For a
 * placeholder it asks for the name instead of the link and links to its
 * code at OpenGTINDB (F31), also when it was scanned again. The card has
 * no time limit; [×] closes it.
 */
export function ResultCard({
  booking,
  disabled,
  nameInputRef,
  onPlusOne,
  onUndo,
  onProductChange,
  onMerge,
  onClose,
}: ResultCardProps) {
  const nameId = useId();
  const view = cardView(booking);
  const naming = asksForName(booking);
  const plusOneClass = booking.kind === "add" ? "bg-accent" : "bg-consume";

  return (
    <section aria-labelledby={nameId} className="rounded-xl bg-surface p-3 shadow">
      <CardTitle id={nameId} title={view.title} onClose={onClose} />
      {view.review && (
        <p className="w-fit rounded bg-warning px-1.5 text-sm font-medium text-ink">
          Bitte prüfen
        </p>
      )}
      {/* The buttons move below the stock when the text is large. */}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <p className="mr-auto text-xl font-bold tabular-nums">{view.stock}</p>
        <button
          type="button"
          disabled={disabled}
          onClick={onPlusOne}
          className={`pressable min-h-11 min-w-14 shrink-0 rounded-lg px-3 text-lg font-semibold text-white disabled:opacity-40 ${plusOneClass}`}
        >
          +1
        </button>
        <button type="button" disabled={disabled} onClick={onUndo} className={undoClass}>
          Rückgängig
        </button>
      </div>
      {(view.isNew || naming) && (
        <div className="mt-3 space-y-3 border-t border-line pt-3">
          {naming && (
            <NameForm
              productId={booking.result.product.id}
              code={placeholderCode(booking)}
              inputRef={nameInputRef}
              onSaved={onProductChange}
            />
          )}
          {view.isNew && (
            <NewProductActions
              product={booking.result.product}
              rename={!naming}
              onProductChange={onProductChange}
              onMerge={onMerge}
            />
          )}
        </div>
      )}
    </section>
  );
}

interface MarkResultCardProps {
  mark: CardMark;
  /** Locks [Rückgängig], while it runs. */
  disabled: boolean;
  onUndo: () => void;
  /** Called by "Karte schließen". */
  onClose: () => void;
}

/**
 * The result card of the scan view in mode mark (docs/plan.md, F15): the
 * product, "vorgemerkt" or "schon auf der Liste", and [Rückgängig] only if
 * this scan marked the product. The card has no time limit; [×] closes it.
 */
export function MarkResultCard({ mark, disabled, onUndo, onClose }: MarkResultCardProps) {
  const nameId = useId();
  const view = markCardView(mark);

  return (
    <section aria-labelledby={nameId} className="rounded-xl bg-surface p-3 shadow">
      <CardTitle id={nameId} title={view.title} onClose={onClose} />
      <div className="flex flex-wrap items-center justify-end gap-2">
        <p className="mr-auto text-xl font-bold text-marked">{view.status}</p>
        {view.undo && (
          <button type="button" disabled={disabled} onClick={onUndo} className={undoClass}>
            Rückgängig
          </button>
        )}
      </div>
    </section>
  );
}

// The name of the product on a card, in full, and [×], which closes the
// card.
function CardTitle({ id, title, onClose }: { id: string; title: string; onClose: () => void }) {
  return (
    <div className="-mt-1.5 -mr-1.5 flex items-start gap-2">
      <h2 id={id} className="min-w-0 flex-1 self-center font-semibold break-words hyphens-auto">
        {title}
      </h2>
      <button
        type="button"
        aria-label="Karte schließen"
        onClick={onClose}
        className="pressable flex size-11 shrink-0 items-center justify-center rounded-full text-ink-tertiary"
      >
        <CloseIcon />
      </button>
    </div>
  );
}

// The name field of a placeholder (docs/plan.md, F31), empty at first.
// "Speichern" and the Enter key send a PATCH with only the trimmed name;
// an empty name cannot be saved. A failure stays below the field until the
// next input or the next save (ADR-0016). Below it a plain link opens the
// page of code at OpenGTINDB in a new tab; StashBert never requests it.
function NameForm({
  productId,
  code,
  inputRef,
  onSaved,
}: {
  productId: string;
  code: string | null;
  inputRef: Ref<HTMLInputElement>;
  onSaved: (product: Product) => void;
}) {
  const queryClient = useQueryClient();
  const rename = useMutation(productUpdateMutation(queryClient));
  const [text, setText] = useState("");
  const inputId = useId();
  const errorId = useId();
  const name = nameToSave(text);

  function save() {
    if (name === null || rename.isPending) {
      return;
    }
    rename.mutate({ id: productId, patch: { name } }, { onSuccess: onSaved });
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <label htmlFor={inputId} className="block text-sm font-medium text-ink-secondary">
        Name
      </label>
      {/* "Speichern" moves below the field when the line is too narrow. */}
      <div className="mt-1 flex flex-wrap justify-end gap-2">
        <input
          ref={inputRef}
          id={inputId}
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            if (rename.isError) {
              rename.reset();
            }
          }}
          placeholder="Wie heißt das Produkt?"
          maxLength={maxNameLength}
          autoComplete="off"
          aria-invalid={rename.isError}
          aria-describedby={rename.isError ? errorId : undefined}
          className="min-h-11 min-w-0 flex-1 basis-48 rounded-lg border border-line-strong bg-surface px-3 py-2 text-base placeholder:text-ink-tertiary aria-[invalid=true]:border-danger"
        />
        <button
          type="submit"
          disabled={name === null || rename.isPending}
          className="pressable min-h-11 shrink-0 rounded-lg bg-accent px-4 font-medium text-white disabled:opacity-40"
        >
          Speichern
        </button>
      </div>
      <p id={errorId} role="status" className="mt-1 min-h-5 text-sm font-medium text-danger">
        {rename.isError ? "Speichern fehlgeschlagen" : ""}
      </p>
      {code !== null && (
        <a
          href={openGtinDbUrl(code)}
          target="_blank"
          rel="noopener noreferrer"
          className={`mt-1 ${actionClass}`}
        >
          {/* The name of the site is not hyphenated, only broken if it does not fit. */}
          <span>
            Bei <span className="hyphens-manual wrap-anywhere">OpenGTINDB</span> nachsehen
          </span>
        </a>
      )}
    </form>
  );
}

// The actions for a new product: the target chips, which send a PATCH
// with only the target, "Name ändern" (only with rename; a placeholder has
// the name field instead) and the merge.
function NewProductActions({
  product,
  rename,
  onProductChange,
  onMerge,
}: {
  product: Product;
  rename: boolean;
  onProductChange: (product: Product) => void;
  onMerge: () => void;
}) {
  const queryClient = useQueryClient();
  const update = useMutation(productUpdateMutation(queryClient));

  // Hides the message of a saved target after a short time.
  const { status, reset } = update;
  useEffect(() => {
    if (status !== "success") {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [status, reset]);

  let notice = "";
  if (status === "success") {
    notice = "Soll gespeichert";
  } else if (status === "error") {
    notice = "Soll nicht gespeichert";
  }

  function chooseTarget(target: number) {
    update.mutate(
      { id: product.id, patch: { target } },
      { onSuccess: onProductChange },
    );
  }

  return (
    <div>
      <div role="group" aria-label="Soll" className="flex flex-wrap items-center gap-2">
        <span className="mr-1 text-sm font-medium text-ink-secondary">Soll</span>
        {targetChoices.map((value) => {
          const pressed = product.target === value;
          return (
            <button
              key={value}
              type="button"
              aria-pressed={pressed}
              disabled={update.isPending}
              onClick={() => chooseTarget(value)}
              className={`pressable size-11 shrink-0 rounded-lg text-lg font-semibold tabular-nums disabled:opacity-40 ${pressed ? "bg-accent text-white" : "bg-fill text-ink"}`}
            >
              {value}
            </button>
          );
        })}
      </div>
      <p
        role="status"
        className={`mt-1 min-h-5 text-sm font-medium ${status === "error" ? "text-danger" : "text-accent"}`}
      >
        {notice}
      </p>
      <div className="mt-1 flex flex-wrap gap-2">
        {rename && (
          <Link
            to={`/produkt/${encodeURIComponent(product.id)}`}
            state={productLinkState("scan")}
            className={actionClass}
          >
            Name ändern
          </Link>
        )}
        <button type="button" onClick={onMerge} className={actionClass}>
          Stattdessen zu vorhandenem Produkt
        </button>
      </div>
    </div>
  );
}
