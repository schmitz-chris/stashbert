import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ChangeEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { Dialog } from "../components/Dialog";
import { problemCode } from "../lib/api/client";
import {
  productDeleteMutation,
  productQuery,
  productUpdateMutation,
} from "../lib/api/queries";
import { diffPatch, toForm, type ProductForm } from "../lib/productForm";
import type { Product } from "../lib/products";
import { sourceNote } from "../lib/sourceNote";

// How long the confirmation "Gespeichert" stays visible.
const noticeDuration = 2000;

const labelClass = "block text-sm font-medium text-stone-700";
const inputClass =
  "mt-1 min-h-11 w-full rounded-lg border border-stone-300 bg-white px-3 py-2 text-base aria-[invalid=true]:border-red-600";
const buttonClass = "min-h-11 rounded-lg px-4 font-medium disabled:opacity-40";
const sourceText = "Daten und Bild: Open Food Facts (ODbL / CC BY-SA 3.0)";

export function ProductPage() {
  const { id = "" } = useParams();
  const product = useQuery(productQuery(id));

  let content = <p className="mt-4 text-stone-500">Produkt wird geladen …</p>;
  if (product.data !== undefined) {
    content = <ProductEditor key={product.data.id} product={product.data} />;
  } else if (problemCode(product.error) === "not_found") {
    content = (
      <>
        <h1 className="mt-4 text-2xl font-semibold">Produkt nicht gefunden</h1>
        <Link
          to="/vorrat"
          className="mt-6 inline-flex min-h-11 items-center rounded-lg bg-emerald-600 px-4 font-medium text-white"
        >
          Zum Vorrat
        </Link>
      </>
    );
  } else if (product.isError && !product.isFetching) {
    content = (
      <div className="mt-4">
        <p className="text-stone-700">Das Produkt konnte nicht geladen werden.</p>
        <button
          type="button"
          onClick={() => void product.refetch()}
          className={`mt-3 bg-emerald-600 text-white ${buttonClass}`}
        >
          Erneut versuchen
        </button>
      </div>
    );
  }

  return (
    <main className="px-4 pt-2 pb-[calc(1.5rem+env(safe-area-inset-bottom))]">
      <Link
        to="/vorrat"
        className="-ml-2 inline-flex min-h-11 items-center px-2 font-medium text-emerald-700"
      >
        <span aria-hidden="true">‹&nbsp;</span>Zurück
      </Link>
      {content}
    </main>
  );
}

function ProductEditor({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const update = useMutation(productUpdateMutation(queryClient));
  const review = useMutation(productUpdateMutation(queryClient));
  const remove = useMutation(productDeleteMutation(queryClient));
  // base is the product the form started from; the patch holds what the
  // user changed since, so fields the user did not touch are never sent.
  const [base, setBase] = useState(product);
  const [form, setForm] = useState(() => toForm(product));
  const [confirmOpen, setConfirmOpen] = useState(false);

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
  if (update.isError) {
    notice =
      problemCode(update.error) === "invalid_request"
        ? "Ungültige Angaben, nicht gespeichert"
        : "Speichern fehlgeschlagen";
  }

  function save() {
    if (!changed || nameMissing) {
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

  // Returns the props that connect an input or a textarea to field.
  function bind(field: keyof ProductForm) {
    return {
      value: form[field],
      onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const { value } = event.target;
        setForm((current) => ({ ...current, [field]: value }));
      },
      className: inputClass,
    };
  }

  return (
    <>
      {product.has_image && (
        <img
          src={`/api/v1/products/${encodeURIComponent(product.id)}/image`}
          alt=""
          className="mt-2 h-48 w-full rounded-xl bg-white object-contain"
        />
      )}
      <h1 className="mt-4 text-2xl font-semibold break-words hyphens-auto">
        {product.name}
      </h1>
      {product.needs_review && (
        <div className="mt-4 flex flex-wrap items-center gap-3 rounded-xl border border-amber-300 bg-amber-50 p-3">
          <p className="w-full text-amber-900">Bitte die Angaben prüfen.</p>
          <button
            type="button"
            disabled={review.isPending}
            onClick={() =>
              review.mutate({ id: product.id, patch: { name: product.name } })
            }
            className={`border border-amber-400 bg-white text-amber-900 ${buttonClass}`}
          >
            Passt so
          </button>
          <p role="status" className="text-sm font-medium text-red-700">
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
          <p id="product-name-hint" className="-mt-3 text-sm text-red-700">
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
        <label className="block">
          <span className={labelClass}>Notiz</span>
          <textarea {...bind("note")} rows={3} maxLength={500} />
        </label>
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            disabled={!changed || update.isPending}
            className={`bg-emerald-600 text-white ${buttonClass}`}
          >
            Speichern
          </button>
          <p
            role="status"
            className={`text-sm font-medium ${update.isError ? "text-red-700" : "text-emerald-700"}`}
          >
            {notice}
          </p>
        </div>
      </form>
      {source !== null && (
        <p className="mt-6 text-sm text-stone-500">
          {source.link === null ? (
            sourceText
          ) : (
            <a
              href={source.link}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex min-h-11 items-center underline"
            >
              {sourceText}
            </a>
          )}
        </p>
      )}
      <button
        type="button"
        onClick={() => {
          remove.reset();
          setConfirmOpen(true);
        }}
        className={`mt-8 w-full border border-red-300 bg-white text-red-700 ${buttonClass}`}
      >
        Produkt löschen
      </button>
      <Dialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title="Produkt löschen?"
      >
        <p className="mt-2 text-stone-700">
          „{product.name}“ wird mit seinen Barcodes und Buchungen gelöscht.
        </p>
        <p role="status" className="mt-2 text-sm font-medium text-red-700">
          {remove.isError ? "Löschen fehlgeschlagen" : ""}
        </p>
        <div className="mt-4 flex justify-end gap-3">
          <button
            type="button"
            onClick={() => setConfirmOpen(false)}
            className={`border border-stone-300 bg-white text-stone-700 ${buttonClass}`}
          >
            Abbrechen
          </button>
          <button
            type="button"
            disabled={remove.isPending}
            onClick={() =>
              remove.mutate(product.id, {
                onSuccess: () => void navigate("/vorrat", { replace: true }),
              })
            }
            className={`bg-red-600 text-white ${buttonClass}`}
          >
            Löschen
          </button>
        </div>
      </Dialog>
    </>
  );
}
