import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { shoppingListQuery } from "../lib/api/queries";
import { shoppingText, type ShoppingItem } from "../lib/shopping";

// How long the notice after sharing stays visible.
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
              <p className="text-stone-700">
                Die Einkaufsliste konnte nicht geladen werden.
              </p>
              <button
                type="button"
                onClick={() => void list.refetch()}
                className="mt-3 min-h-11 rounded-lg bg-emerald-600 px-4 font-medium text-white"
              >
                Erneut versuchen
              </button>
            </div>
          ) : (
            <p className="text-stone-500">Einkaufsliste wird geladen …</p>
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
    return <p className="text-stone-500">Alles da</p>;
  }
  return (
    <>
      <ul className="divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white">
        {items.map((item) => (
          <li key={item.product_id}>
            <Link
              to={`/produkt/${encodeURIComponent(item.product_id)}`}
              className="flex min-h-11 flex-col justify-center px-3 py-2"
            >
              <span className="font-medium break-words hyphens-auto">
                <span className="tabular-nums">{item.missing}</span> ×{" "}
                {item.name}
              </span>
              {item.brand !== null && (
                <span className="text-sm break-words hyphens-auto text-stone-500">
                  {item.brand}
                </span>
              )}
            </Link>
          </li>
        ))}
      </ul>
      <ShareButton items={items} />
    </>
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
        className="min-h-11 w-full rounded-lg bg-emerald-600 px-4 font-medium text-white"
      >
        Als Text teilen
      </button>
      <p
        role="status"
        className={`mt-2 text-sm font-medium ${notice === "failed" ? "text-red-700" : "text-emerald-700"}`}
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
