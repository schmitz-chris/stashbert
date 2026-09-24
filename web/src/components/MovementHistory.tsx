import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { productMovementsQuery } from "../lib/api/queries";
import {
  formatDelta,
  formatMovementTime,
  movementKindLabel,
  visibleMovements,
  type Movement,
} from "../lib/movements";

/**
 * Lists the latest movements of the product with productId: three, and
 * with "Alle anzeigen" all that the query loads (ten).
 */
export function MovementHistory({ productId }: { productId: string }) {
  const movements = useQuery(productMovementsQuery(productId));
  const [expanded, setExpanded] = useState(false);
  const headingId = useId();

  let content = <p className="mt-2 text-ink-tertiary">Verlauf wird geladen …</p>;
  if (movements.data !== undefined) {
    const shown = visibleMovements(movements.data, expanded);
    content =
      movements.data.length === 0 ? (
        <p className="mt-2 text-ink-tertiary">Noch keine Buchungen.</p>
      ) : (
        <>
          <ul className="mt-2 divide-y divide-line rounded-xl border border-line bg-surface">
            {shown.map((movement) => (
              <MovementRow key={movement.id} movement={movement} />
            ))}
          </ul>
          {shown.length < movements.data.length && (
            <button
              type="button"
              onClick={() => setExpanded(true)}
              className="pressable mt-1 -ml-2 min-h-11 rounded-lg px-2 font-medium text-accent"
            >
              Alle anzeigen
            </button>
          )}
        </>
      );
  } else if (movements.isError && !movements.isFetching) {
    content = (
      <div className="mt-2">
        <p className="text-ink-secondary">Der Verlauf konnte nicht geladen werden.</p>
        <button
          type="button"
          onClick={() => void movements.refetch()}
          className="pressable mt-3 min-h-11 rounded-lg border border-line-strong bg-surface px-4 font-medium text-accent"
        >
          Erneut versuchen
        </button>
      </div>
    );
  }

  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="text-lg font-semibold">
        Letzte Buchungen
      </h2>
      {content}
    </section>
  );
}

function MovementRow({ movement }: { movement: Movement }) {
  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2">
      <div>
        <p className="font-medium">{movementKindLabel(movement.kind)}</p>
        <time dateTime={movement.created_at} className="text-sm text-ink-tertiary">
          {formatMovementTime(movement.created_at)}
        </time>
      </div>
      <span className="shrink-0 text-lg font-medium tabular-nums">
        {formatDelta(movement.delta)}
      </span>
    </li>
  );
}
