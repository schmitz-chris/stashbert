import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useReducer, useState } from "react";
import { Link } from "react-router";
import { CartButton } from "../components/CartButton";
import { CloseIcon } from "../components/CloseIcon";
import { PageHeading } from "../components/PageHeading";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import {
  markProductMutation,
  shoppingListQuery,
  unmarkMutation,
} from "../lib/api/queries";
import { pageTitle } from "../lib/pageTitle";
import { productLinkState } from "../lib/productOrigin";
import {
  shoppingQuantity,
  shoppingText,
  type ShoppingItem,
} from "../lib/shopping";
import {
  hiddenUndoBar,
  undoBarReducer,
  type RemovedItem,
  type UndoBarAction,
  type UndoBarState,
} from "../lib/undoBar";

// How long the notice after copying the list stays visible. Failures stay
// until the next action (ADR-0016).
const noticeDuration = 2000;

// How often the undo bar checks whether its time is up, in ms.
const undoBarTickInterval = 200;

// The notice after sharing: none, the text was copied, or sharing failed.
type ShareNotice = "" | "copied" | "failed";

const noticeText: Record<ShareNotice, string> = {
  "": "",
  copied: "In die Zwischenablage kopiert",
  failed: "Teilen fehlgeschlagen",
};

export function ShoppingPage() {
  const list = useQuery(shoppingListQuery);
  const [bar, dispatchBar] = useReducer(undoBarReducer, hiddenUndoBar);
  useDocumentTitle(pageTitle("shopping"));

  // While the bar is shown, reports the time to the reducer, which hides
  // the bar when its time is up.
  const barRunning = bar.status === "shown";
  useEffect(() => {
    if (!barRunning) {
      return;
    }
    const timer = setInterval(
      () => dispatchBar({ type: "tick", now: Date.now() }),
      undoBarTickInterval,
    );
    return () => clearInterval(timer);
  }, [barRunning]);

  return (
    <>
      <PageHeading className="text-2xl font-semibold">Einkauf</PageHeading>
      <div className="mt-4">
        {list.data === undefined ? (
          list.isError && !list.isFetching ? (
            <div>
              <p className="text-ink-secondary">
                Die Einkaufsliste konnte nicht geladen werden.
              </p>
              <button
                type="button"
                onClick={() => void list.refetch()}
                className="pressable mt-3 min-h-11 rounded-lg bg-accent px-4 font-medium text-white"
              >
                Erneut versuchen
              </button>
            </div>
          ) : (
            <p className="text-ink-tertiary">Einkaufsliste wird geladen …</p>
          )
        ) : (
          <ShoppingList
            items={list.data}
            onRemoved={(item) => dispatchBar({ type: "removed", item, now: Date.now() })}
          />
        )}
      </div>
      <UndoBar state={bar} dispatch={dispatchBar} />
    </>
  );
}

function ShoppingList({
  items,
  onRemoved,
}: {
  items: ShoppingItem[];
  onRemoved: (item: RemovedItem) => void;
}) {
  if (items.length === 0) {
    return <p className="text-ink-tertiary">Alles da</p>;
  }
  return (
    <>
      <ul className="overflow-hidden rounded-xl bg-surface">
        {items.map((item) => (
          <ShoppingRow key={item.product_id} item={item} onRemoved={onRemoved} />
        ))}
      </ul>
      <ShareButton items={items} />
    </>
  );
}

function ShoppingRow({
  item,
  onRemoved,
}: {
  item: ShoppingItem;
  onRemoved: (item: RemovedItem) => void;
}) {
  const queryClient = useQueryClient();
  const unmark = useMutation(unmarkMutation(queryClient));
  const quantity = shoppingQuantity(item);

  // The link covers the whole row with its ::after box, so a tap anywhere
  // outside the button opens the product. The button lies above it. The
  // separator above a row (::before) starts where the text starts, like in
  // the stock list.
  return (
    <li className="relative flex min-h-11 items-center gap-3 px-4 py-3 before:absolute before:top-0 before:right-0 before:left-4 before:border-t before:border-line first:before:hidden">
      <div className="min-w-0 flex-1">
        <Link
          to={`/produkt/${encodeURIComponent(item.product_id)}`}
          state={productLinkState("einkauf")}
          className="pressable-row font-medium break-words hyphens-auto after:absolute after:inset-0"
        >
          {quantity !== null && (
            <>
              <span className="tabular-nums">{quantity}</span> ×{" "}
            </>
          )}
          {item.name}
        </Link>
        {item.brand !== null && (
          <p className="text-sm break-words hyphens-auto text-ink-tertiary">
            {item.brand}
          </p>
        )}
        {item.marked && (
          <p className="text-xs font-medium text-marked">vorgemerkt</p>
        )}
        <p role="status" className="text-sm font-medium text-danger">
          {unmark.isError ? "Entfernen fehlgeschlagen" : ""}
        </p>
      </div>
      {item.marked && (
        <CartButton
          checked
          label={`Von der Liste nehmen: ${item.name}`}
          disabled={unmark.isPending}
          onClick={() =>
            unmark.mutate(item.product_id, {
              onSuccess: () => onRemoved({ productId: item.product_id, name: item.name }),
            })
          }
        />
      )}
    </li>
  );
}

// The bar after removing an item: "Entfernt: <Name>" and [Rückgängig],
// which marks the product again; after a failed undo also [×], which
// closes it. It floats above the navigation bar (64 px high, the scan
// button rises 12 px above it) and its safe area, and leaves the rest of
// the screen tappable.
function UndoBar({
  state,
  dispatch,
}: {
  state: UndoBarState;
  dispatch: (action: UndoBarAction) => void;
}) {
  const queryClient = useQueryClient();
  const mark = useMutation(markProductMutation(queryClient));

  function undo(productId: string) {
    dispatch({ type: "undo" });
    mark.mutate(productId, {
      onSuccess: () => dispatch({ type: "undone", productId }),
      onError: () => dispatch({ type: "undoFailed", productId }),
    });
  }

  return (
    <div
      role="status"
      className="pointer-events-none fixed inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+88px)] z-30 pr-[max(1rem,env(safe-area-inset-right))] pl-[max(1rem,env(safe-area-inset-left))]"
    >
      {state.status !== "hidden" && (
        <div className="pointer-events-auto mx-auto flex max-w-md flex-wrap items-center justify-end gap-x-3 gap-y-1 rounded-xl border border-line bg-surface py-2 pr-2 pl-4 shadow-lg">
          <p
            className={`min-w-[8rem] flex-1 break-words hyphens-auto ${state.status === "failed" ? "font-medium text-danger" : "text-ink"}`}
          >
            {state.status === "failed"
              ? `Rückgängig fehlgeschlagen: ${state.item.name}`
              : `Entfernt: ${state.item.name}`}
          </p>
          <button
            type="button"
            disabled={state.status === "undoing"}
            onClick={() => undo(state.item.productId)}
            className="pressable min-h-11 shrink-0 rounded-lg bg-fill px-3 font-medium text-ink disabled:opacity-40"
          >
            Rückgängig
          </button>
          {state.status === "failed" && (
            <button
              type="button"
              aria-label="Meldung schließen"
              onClick={() => dispatch({ type: "close" })}
              className="pressable flex size-11 shrink-0 items-center justify-center rounded-lg text-ink-tertiary"
            >
              <CloseIcon />
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function ShareButton({ items }: { items: ShoppingItem[] }) {
  const [notice, setNotice] = useState<ShareNotice>("");

  // Hides the notice after copying after a short time; a failure stays
  // until the next tap.
  useEffect(() => {
    if (notice !== "copied") {
      return;
    }
    const timer = setTimeout(() => setNotice(""), noticeDuration);
    return () => clearTimeout(timer);
  }, [notice]);

  async function share() {
    setNotice("");
    setNotice(await shareOrCopy(shoppingText(items)));
  }

  return (
    <div className="mt-4">
      <button
        type="button"
        onClick={() => void share()}
        className="pressable min-h-11 w-full rounded-lg bg-accent px-4 font-medium text-white"
      >
        Als Text teilen
      </button>
      <p
        role="status"
        className={`mt-2 text-sm font-medium ${notice === "failed" ? "text-danger" : "text-accent"}`}
      >
        {noticeText[notice]}
      </p>
    </div>
  );
}

// Shares text with the Web Share API. A share the user cancels
// (AbortError) gives no notice, any other error of the share gives
// "failed". Without the Share API the text is copied to the clipboard
// instead. Returns the notice to show.
async function shareOrCopy(text: string): Promise<ShareNotice> {
  if (typeof navigator.share === "function") {
    try {
      await navigator.share({ text });
      return "";
    } catch (error) {
      return error instanceof DOMException && error.name === "AbortError"
        ? ""
        : "failed";
    }
  }
  try {
    await navigator.clipboard.writeText(text);
    return "copied";
  } catch {
    return "failed";
  }
}
