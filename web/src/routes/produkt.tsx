import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState, type ChangeEvent } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import { ConfirmActions, Dialog } from "../components/Dialog";
import { MergeSection } from "../components/MergeSection";
import { MovementHistory } from "../components/MovementHistory";
import { PageHeading } from "../components/PageHeading";
import { ProductImageSection } from "../components/ProductImageSection";
import { ShoppingListSection } from "../components/ShoppingListSection";
import { StockSection } from "../components/StockSection";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { problemCode } from "../lib/api/client";
import {
  barcodeAddMutation,
  barcodeRemoveMutation,
  productDeleteMutation,
  productQuery,
  productUpdateMutation,
} from "../lib/api/queries";
import { normalizeGtin } from "../lib/gtin";
import { pageTitle } from "../lib/pageTitle";
import { diffPatch, toForm, withCrate, type ProductForm } from "../lib/productForm";
import { productBack } from "../lib/productOrigin";
import type { Product } from "../lib/products";
import { sourceNote } from "../lib/sourceNote";

// How long the confirmation "Gespeichert" stays visible.
const noticeDuration = 2000;

const labelClass = "block text-sm font-medium text-ink-secondary";
const inputClass =
  "mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 py-2 text-base aria-[invalid=true]:border-danger";
const buttonClass = "pressable min-h-11 rounded-lg px-4 font-medium disabled:opacity-40";
// Every action besides "Speichern", the one filled button of the page
// (docs/plan.md, F22).
const secondaryClass = `border border-line-strong bg-surface ${buttonClass}`;
// A row that opens and closes a details element (HIG, Disclosure controls):
// 44 px high over the full width, the chevron on the trailing edge.
const summaryClass =
  "pressable flex min-h-11 cursor-pointer list-none items-center gap-3 py-1 text-lg font-semibold [&::-webkit-details-marker]:hidden";
const sourceText = "Daten und Bild: Open Food Facts (ODbL / CC BY-SA 3.0)";

export function ProductPage() {
  const { id = "" } = useParams();
  const product = useQuery(productQuery(id));
  const navigate = useNavigate();
  const location = useLocation();
  // The first location of a visit has the key "default". Only then is there
  // no page of the app to go back to (reload, start of the installed app).
  const canGoBack = location.key !== "default";
  const back = productBack(location.state);
  const notFound = problemCode(product.error) === "not_found";
  useDocumentTitle(
    notFound ? pageTitle("not-found") : pageTitle("product", product.data?.name),
  );

  let content = <p className="mt-4 text-ink-tertiary">Produkt wird geladen …</p>;
  if (product.data !== undefined) {
    content = <ProductEditor key={product.data.id} product={product.data} />;
  } else if (notFound) {
    content = (
      <>
        <PageHeading className="mt-4 text-2xl font-semibold">
          Produkt nicht gefunden
        </PageHeading>
        <Link
          to="/vorrat"
          className="pressable mt-6 inline-flex min-h-11 items-center rounded-lg bg-accent px-4 font-medium text-white"
        >
          Zum Vorrat
        </Link>
      </>
    );
  } else if (product.isError && !product.isFetching) {
    content = (
      <div className="mt-4">
        <p className="text-ink-secondary">Das Produkt konnte nicht geladen werden.</p>
        <button
          type="button"
          onClick={() => void product.refetch()}
          className={`mt-3 bg-accent text-white ${buttonClass}`}
        >
          Erneut versuchen
        </button>
      </div>
    );
  }

  return (
    <>
      {/* The navigation bar stays at the top below the safe area, over the
          full width of the view (it takes back the padding of main). It lies
          above the page and below the update banner and the tab bar (z-20). */}
      <header className="sticky top-[env(safe-area-inset-top)] z-15 -mx-4 -mt-6 grid grid-cols-[1fr_minmax(0,max-content)_1fr] items-center gap-2 border-b border-line bg-canvas px-4">
        <Link
          to={back.path}
          onClick={(event) => {
            // Back to where the product was opened, e.g. Einkauf or Scan.
            if (canGoBack) {
              event.preventDefault();
              void navigate(-1);
            }
          }}
          aria-label={`Zurück: ${back.label}`}
          className="pressable -ml-2 inline-flex min-h-11 items-center justify-self-start rounded-lg px-2 font-medium whitespace-nowrap text-accent"
        >
          <span aria-hidden="true">‹&nbsp;</span>
          {back.label}
        </Link>
        {/* Screen readers read the name in the h1 below. */}
        <p aria-hidden="true" className="truncate text-center font-semibold">
          {product.data?.name}
        </p>
      </header>
      {content}
    </>
  );
}

function ProductEditor({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const update = useMutation(productUpdateMutation(queryClient));
  const review = useMutation(productUpdateMutation(queryClient));
  // base is the product the form started from; the patch holds what the
  // user changed since, so fields the user did not touch are never sent.
  const [base, setBase] = useState(product);
  const [form, setForm] = useState(() => toForm(product));
  // Set when "Speichern" was chosen without a change, until the next one.
  const [unchanged, setUnchanged] = useState(false);

  const patch = diffPatch(base, form);
  const changed = Object.keys(patch).length > 0;
  // Takes over a newer version of the product (a refetch or "Passt so"),
  // as long as the form has no unsaved changes.
  if (product.updated_at !== base.updated_at && !changed) {
    setBase(product);
    setForm(toForm(product));
  }
  const nameMissing = form.name.trim() === "";
  const source = sourceNote(product);

  // Hides the confirmation of a successful save after a short time.
  const { isSuccess, reset } = update;
  useEffect(() => {
    if (!isSuccess) {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [isSuccess, reset]);

  let notice = isSuccess ? "Gespeichert" : "";
  let noticeClass = "text-accent";
  if (unchanged) {
    notice = "Keine Änderungen";
    noticeClass = "text-ink-secondary";
  }
  if (update.isError) {
    notice =
      problemCode(update.error) === "invalid_request"
        ? "Ungültige Angaben, nicht gespeichert"
        : "Speichern fehlgeschlagen";
    noticeClass = "text-danger";
  }

  // "Speichern" stays enabled: without a change it says so, without a
  // name the hint below the field says why.
  function save() {
    if (nameMissing) {
      return;
    }
    if (!changed) {
      update.reset();
      setUnchanged(true);
      return;
    }
    update.mutate(
      { id: product.id, patch },
      {
        onSuccess: (saved) => {
          setBase(saved);
          setForm(toForm(saved));
        },
      },
    );
  }

  // Returns the props that connect an input or a textarea to field, a text
  // of the form (every field but the checkbox crate).
  function bind(field: Exclude<keyof ProductForm, "crate">) {
    return {
      value: form[field],
      onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const { value } = event.target;
        setUnchanged(false);
        setForm((current) => ({ ...current, [field]: value }));
      },
      className: inputClass,
    };
  }

  return (
    <>
      <PageHeading className="mt-4 text-2xl font-semibold break-words hyphens-auto">
        {product.name}
      </PageHeading>
      {product.brand !== null && (
        <p className="break-words hyphens-auto text-ink-tertiary">{product.brand}</p>
      )}
      <ProductImageSection product={product} />
      {product.needs_review && (
        <div className="mt-4 flex flex-wrap items-center gap-3 rounded-xl border border-warning bg-warning-soft p-3">
          <p className="w-full text-ink">Bitte die Angaben prüfen.</p>
          <button
            type="button"
            disabled={review.isPending}
            onClick={() =>
              review.mutate({ id: product.id, patch: { name: product.name } })
            }
            className={`text-accent ${secondaryClass}`}
          >
            Passt so
          </button>
          <MergeSection product={product} />
          <p role="status" className="text-sm font-medium text-danger">
            {review.isError ? "Speichern fehlgeschlagen" : ""}
          </p>
        </div>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
        className="mt-6 space-y-4"
      >
        <label className="block">
          <span className={labelClass}>Name</span>
          <input
            {...bind("name")}
            maxLength={120}
            aria-invalid={nameMissing}
            aria-describedby={nameMissing ? "product-name-hint" : undefined}
          />
        </label>
        {nameMissing && (
          <p id="product-name-hint" className="-mt-3 text-sm text-danger">
            Bitte einen Namen eingeben.
          </p>
        )}
        <label className="block">
          <span className={labelClass}>Marke</span>
          <input {...bind("brand")} maxLength={120} />
        </label>
        <label className="block">
          <span className={labelClass}>Packungsgröße</span>
          <input {...bind("package_size")} maxLength={40} />
        </label>
        <label className="block">
          <span className={labelClass}>Soll</span>
          <input
            {...bind("target")}
            type="number"
            inputMode="numeric"
            required
            min={0}
            max={100000}
            step={1}
          />
        </label>
        {/* Only a product with a crate size asks "Flasche oder Kasten" when
            storing (ADR-0017). The whole row, 44 px high, toggles it. */}
        <label className="pressable -mx-2 flex min-h-11 cursor-pointer items-center gap-3 rounded-lg px-2">
          <input
            type="checkbox"
            checked={form.crate}
            onChange={(event) => {
              const { checked } = event.target;
              setUnchanged(false);
              setForm((current) => withCrate(current, checked));
            }}
            className="size-5 shrink-0 accent-accent"
          />
          <span className="font-medium">Kastenware</span>
        </label>
        {form.crate && (
          <label className="block">
            <span className={labelClass}>Flaschen pro Kasten</span>
            <input
              {...bind("crate_size")}
              type="number"
              inputMode="numeric"
              required
              min={2}
              max={100}
              step={1}
            />
          </label>
        )}
        <label className="block">
          <span className={labelClass}>Notiz</span>
          <textarea {...bind("note")} rows={3} maxLength={500} />
        </label>
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            disabled={update.isPending}
            className={`bg-accent text-white ${buttonClass}`}
          >
            Speichern
          </button>
          <p role="status" className={`text-sm font-medium ${noticeClass}`}>
            {notice}
          </p>
        </div>
      </form>
      <StockSection product={product} />
      <ShoppingListSection product={product} />
      <MovementHistory productId={product.id} />
      {source !== null && (
        <p className="mt-6 text-sm text-ink-tertiary">
          {source.link === null ? (
            sourceText
          ) : (
            <a
              href={source.link}
              target="_blank"
              rel="noopener noreferrer"
              className="pressable inline-flex min-h-11 items-center rounded-lg underline"
            >
              {sourceText}
            </a>
          )}
        </p>
      )}
      {/* Progressive disclosure (HIG, Layout): what is rarely needed. */}
      <div className="mt-8 border-y border-line">
        <BarcodeSection product={product} />
        <DeleteSection product={product} />
      </div>
    </>
  );
}

// The chevron of a summary: it points to the trailing edge while the
// details are closed and down while they are open.
function Chevron() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-5 shrink-0 text-accent group-open:rotate-90 motion-safe:transition-transform"
    >
      <path d="m9 5 7 7-7 7" />
    </svg>
  );
}

// "Produkt löschen", closed at first, and its confirmation.
function DeleteSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const remove = useMutation(productDeleteMutation(queryClient));
  const [confirmOpen, setConfirmOpen] = useState(false);

  return (
    <>
      <details className="group border-t border-line">
        <summary className={summaryClass}>
          <span className="min-w-0 flex-1">Produkt löschen</span>
          <Chevron />
        </summary>
        <button
          type="button"
          onClick={() => {
            remove.reset();
            setConfirmOpen(true);
          }}
          className={`mt-1 mb-4 w-full text-danger ${secondaryClass}`}
        >
          Produkt löschen
        </button>
      </details>
      <Dialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title="Produkt löschen?"
      >
        <p className="mt-2 text-ink-secondary">
          „{product.name}“ wird mit seinen Barcodes und Buchungen gelöscht.
        </p>
        <p role="status" className="mt-2 text-sm font-medium text-danger">
          {remove.isError ? "Löschen fehlgeschlagen" : ""}
        </p>
        <ConfirmActions
          label="Löschen"
          pending={remove.isPending}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={() =>
            remove.mutate(product.id, {
              onSuccess: () => void navigate("/vorrat", { replace: true }),
            })
          }
        />
      </Dialog>
    </>
  );
}

function BarcodeSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const add = useMutation(barcodeAddMutation(queryClient));
  const remove = useMutation(barcodeRemoveMutation(queryClient));
  const [code, setCode] = useState("");
  // localHint is set when the field is empty, the entered code fails
  // normalizeGtin or already belongs to this product; then no request is
  // sent.
  const [localHint, setLocalHint] = useState<string | null>(null);
  // The code of the barcode whose removal the dialog asks to confirm.
  const [removing, setRemoving] = useState<string | null>(null);
  const inputId = useId();
  const hintId = useId();

  const errorCode = add.isError ? problemCode(add.error) : undefined;
  const codeRejected =
    localHint !== null ||
    errorCode === "invalid_barcode" ||
    errorCode === "barcode_in_use";
  let hint = "";
  if (localHint !== null) {
    hint = localHint;
  } else if (errorCode === "invalid_barcode") {
    hint = "Ungültiger Barcode";
  } else if (errorCode === "barcode_in_use") {
    hint = "Barcode gehört schon zu einem anderen Produkt";
  } else if (add.isError) {
    hint = "Hinzufügen fehlgeschlagen";
  }

  function submit() {
    // "Hinzufügen" stays enabled; with an empty field it says why.
    if (code.trim() === "") {
      add.reset();
      setLocalHint("Bitte einen Barcode eingeben");
      return;
    }
    const normalized = normalizeGtin(code.trim());
    if (normalized === null) {
      add.reset();
      setLocalHint("Ungültiger Barcode");
      return;
    }
    // The server answers barcode_in_use here too, which would wrongly
    // name another product.
    if (product.barcodes.some((barcode) => barcode.code === normalized)) {
      add.reset();
      setLocalHint("Barcode ist diesem Produkt schon zugeordnet");
      return;
    }
    setLocalHint(null);
    add.mutate(
      { productId: product.id, code: normalized },
      { onSuccess: () => setCode("") },
    );
  }

  function confirmRemove(removed: string) {
    remove.mutate(
      { productId: product.id, code: removed },
      {
        onSuccess: () => setRemoving(null),
        // The barcode is gone already; the product is fetched again.
        onError: (error) => {
          if (problemCode(error) === "not_found") {
            setRemoving(null);
          }
        },
      },
    );
  }

  return (
    <>
      <details className="group">
        <summary className={summaryClass}>
          <span className="min-w-0 flex-1">Barcodes</span>
          <span className="font-normal text-ink-tertiary tabular-nums">
            {product.barcodes.length}
          </span>
          <Chevron />
        </summary>
        {product.barcodes.length === 0 ? (
          <p className="mt-1 text-ink-tertiary">Noch keine Barcodes.</p>
        ) : (
          <ul className="mt-1 divide-y divide-line overflow-hidden rounded-xl border border-line bg-surface">
            {product.barcodes.map((barcode) => (
              <li
                key={barcode.code}
                className="flex items-center justify-between gap-3 pl-3"
              >
                <span className="font-mono break-all">{barcode.code}</span>
                <button
                  type="button"
                  onClick={() => {
                    remove.reset();
                    setRemoving(barcode.code);
                  }}
                  aria-label={`Barcode ${barcode.code} entfernen`}
                  className="pressable min-h-11 min-w-11 shrink-0 px-3 font-medium text-danger"
                >
                  Entfernen
                </button>
              </li>
            ))}
          </ul>
        )}
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
          className="mt-4 mb-3"
        >
          <label htmlFor={inputId} className={labelClass}>
            Barcode hinzufügen
          </label>
          {/* The button moves below the field when the text is large. */}
          <div className="mt-1 flex flex-wrap gap-2">
            <input
              id={inputId}
              value={code}
              onChange={(event) => {
                setCode(event.target.value);
                setLocalHint(null);
                if (add.isError) {
                  add.reset();
                }
              }}
              inputMode="numeric"
              autoComplete="off"
              aria-invalid={codeRejected}
              aria-describedby={hint !== "" ? hintId : undefined}
              className="min-h-11 min-w-[8rem] flex-1 rounded-lg border border-line-strong bg-surface px-3 py-2 font-mono text-base aria-[invalid=true]:border-danger"
            />
            <button
              type="submit"
              disabled={add.isPending}
              className={`shrink-0 text-accent ${secondaryClass}`}
            >
              Hinzufügen
            </button>
          </div>
          <p
            id={hintId}
            role="status"
            className="mt-1 text-sm font-medium text-danger"
          >
            {hint}
          </p>
        </form>
      </details>
      <Dialog
        open={removing !== null}
        onClose={() => setRemoving(null)}
        title="Barcode entfernen?"
      >
        <p className="mt-2 text-ink-secondary">
          Der Barcode <span className="font-mono">{removing}</span> gehört dann
          nicht mehr zu „{product.name}“.
        </p>
        <p role="status" className="mt-2 text-sm font-medium text-danger">
          {remove.isError && problemCode(remove.error) !== "not_found"
            ? "Entfernen fehlgeschlagen"
            : ""}
        </p>
        <ConfirmActions
          label="Entfernen"
          pending={remove.isPending}
          onCancel={() => setRemoving(null)}
          onConfirm={() => {
            if (removing !== null) {
              confirmRemove(removing);
            }
          }}
        />
      </Dialog>
    </>
  );
}
