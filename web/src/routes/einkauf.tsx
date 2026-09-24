import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { shoppingListQuery, unmarkMutation } from "../lib/api/queries";
import {
  shoppingQuantity,
  shoppingText,
  type ShoppingItem,
} from "../lib/shopping";

// How long the notice after sharing or after a failed removal stays visible.
const noticeDuration = 2000;

// The notice after sharing: none, the text was copied, or sharing failed.
type ShareNotice = "" | "copied" | "failed";

const noticeText: Record<ShareNotice, string> = {
  "": "",
  copied: "In die Zwischenablage kopiert",
  failed: "Teilen fehlgeschlagen",
};

export function ShoppingPage() {
  const list = useQuery(shoppingListQuery);

  return (
    <>
      <h1 className="text-2xl font-semibold">Einkauf</h1>
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
          <ShoppingList items={list.data} />
        )}
      </div>
    </>
  );
}

function ShoppingList({ items }: { items: ShoppingItem[] }) {
  if (items.length === 0) {
    return <p className="text-ink-tertiary">Alles da</p>;
  }
  return (
    <>
      <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line bg-surface">
        {items.map((item) => (
          <ShoppingRow key={item.product_id} item={item} />
        ))}
      </ul>
      <ShareButton items={items} />
    </>
  );
}

function ShoppingRow({ item }: { item: ShoppingItem }) {
  const queryClient = useQueryClient();
  const unmark = useMutation(unmarkMutation(queryClient));
  const { isError, reset } = unmark;
  const quantity = shoppingQuantity(item);

  // Hides the notice of a failed removal after a short time.
  useEffect(() => {
    if (!isError) {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [isError, reset]);

  // The link covers the whole row with its ::after box, so a tap anywhere
  // outside the button opens the product. The button lies above it.
  return (
    <li className="relative flex min-h-11 items-center gap-2 px-3 py-2">
      <div className="min-w-0 flex-1">
        <Link
          to={`/produkt/${encodeURIComponent(item.product_id)}`}
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
          {isError ? "Entfernen fehlgeschlagen" : ""}
        </p>
      </div>
      {item.marked && (
        <button
          type="button"
          disabled={unmark.isPending}
          onClick={() => unmark.mutate(item.product_id)}
          aria-label={`Von der Liste nehmen: ${item.name}`}
          className="pressable relative z-10 min-h-11 shrink-0 rounded-lg border border-line-strong bg-surface px-3 text-sm font-medium text-ink-secondary disabled:opacity-40"
        >
          Von der Liste nehmen
        </button>
      )}
    </li>
  );
}

function ShareButton({ items }: { items: ShoppingItem[] }) {
  const [notice, setNotice] = useState<ShareNotice>("");

  // Hides the notice after a short time.
  useEffect(() => {
    if (notice === "") {
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
