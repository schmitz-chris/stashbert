import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, type ReactNode } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { PageHeading } from "../components/PageHeading";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { problemCode } from "../lib/api/client";
import {
  mqttStatusQuery,
  shoppingSnapshotMutation,
  shoppingTargetMutation,
  systemStatusQuery,
} from "../lib/api/queries";
import { pageTitle } from "../lib/pageTitle";
import {
  backupCountText,
  formatSize,
  lastBackupText,
  mqttStateText,
  openFoodFactsText,
} from "../lib/settings";
import { noTarget, targetChoice, type MqttStatus } from "../lib/shoppingTarget";

// How long the notice after sending the list stays visible. Failures stay
// until the next action (ADR-0016).
const noticeDuration = 2000;

// Secondary buttons (docs/plan.md, F22).
const secondaryButtonClass =
  "pressable min-h-11 rounded-lg border border-line-strong bg-surface px-4 py-2 font-medium text-accent disabled:opacity-40";

// The download of a fresh backup. A download is navigation, not an API
// call, so it is the one address of the API outside the generated client
// (docs/plan.md, F32).
const backupUrl = "/api/v1/backup";

/**
 * The settings (docs/plan.md, F32), opened by the gear in the stock view:
 * Home Assistant, the backups and information about StashBert. Connection
 * data stays in the configuration file of the server; the page only shows
 * whether it is set (architecture.md, 4.2).
 */
export function SettingsPage() {
  const navigate = useNavigate();
  const location = useLocation();
  // The first location of a visit has the key "default". Only then is there
  // no page of the app to go back to (reload, start of the installed app).
  const canGoBack = location.key !== "default";
  useDocumentTitle(pageTitle("settings"));

  return (
    <>
      {/* The navigation bar of the product page: it stays at the top below
          the safe area, over the full width of the view. */}
      <header className="sticky top-[env(safe-area-inset-top)] z-15 -mx-4 -mt-6 grid grid-cols-[1fr_minmax(0,max-content)_1fr] items-center gap-2 border-b border-line bg-canvas px-4">
        <Link
          to="/vorrat"
          onClick={(event) => {
            if (canGoBack) {
              event.preventDefault();
              void navigate(-1);
            }
          }}
          aria-label="Zurück: Vorrat"
          className="pressable -ml-2 inline-flex min-h-11 items-center justify-self-start rounded-lg px-2 font-medium whitespace-nowrap text-accent"
        >
          <span aria-hidden="true">‹&nbsp;</span>
          Vorrat
        </Link>
        {/* Screen readers read the title in the h1 below. */}
        <p aria-hidden="true" className="truncate text-center font-semibold">
          Einstellungen
        </p>
      </header>
      <PageHeading className="mt-4 text-2xl font-semibold break-words hyphens-auto">
        Einstellungen
      </PageHeading>
      <HomeAssistantSection />
      <BackupSection />
      <AboutSection />
    </>
  );
}

// A section of the page with its heading.
function Section({ title, children }: { title: string; children: ReactNode }) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="text-lg font-semibold">
        {title}
      </h2>
      {children}
    </section>
  );
}

// The white card of a section.
function Card({ children }: { children: ReactNode }) {
  return <div className="mt-2 rounded-xl bg-surface px-4 py-3">{children}</div>;
}

// What a card shows while its data is not there: that it is loading, or
// that it failed, with a button that loads it again.
function LoadState({ failed, onRetry }: { failed: boolean; onRetry: () => void }) {
  if (!failed) {
    return <p className="text-ink-tertiary">Wird geladen …</p>;
  }
  return (
    <>
      <p className="text-ink-secondary">Konnte nicht geladen werden.</p>
      <button
        type="button"
        onClick={onRetry}
        className="pressable mt-3 min-h-11 rounded-lg bg-accent px-4 font-medium text-white"
      >
        Erneut versuchen
      </button>
    </>
  );
}

// Labels with their values, one row each, like the information in the
// settings of iOS: the value on the right, or below the label if both do
// not fit side by side (large text, narrow screen). The separator above a
// row starts where the text starts.
function Rows({ rows }: { rows: [label: string, value: string][] }) {
  return (
    <dl className="mt-2 rounded-xl bg-surface">
      {rows.map(([label, value]) => (
        <div
          key={label}
          className="relative flex flex-wrap justify-between gap-x-4 px-4 py-3 before:absolute before:top-0 before:right-0 before:left-4 before:border-t before:border-line first:before:hidden"
        >
          <dt>{label}</dt>
          <dd className="break-words text-ink-secondary">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * The section "Home Assistant" (docs/plan.md, F30 and F32): without MQTT
 * "Nicht eingerichtet" and where it is set up; with MQTT the state of the
 * connection, the choice of the list in Home Assistant and "Liste neu
 * senden".
 */
function HomeAssistantSection() {
  const mqtt = useQuery(mqttStatusQuery);
  const status = mqtt.data;

  let content: ReactNode;
  if (status === undefined) {
    content = (
      <LoadState
        failed={mqtt.isError && !mqtt.isFetching}
        onRetry={() => void mqtt.refetch()}
      />
    );
  } else if (status.status === "disabled") {
    content = (
      <>
        <p className="text-ink-secondary">{mqttStateText(status.status)}</p>
        <p className="mt-1 text-sm break-words text-ink-tertiary">
          Die Verbindung wird in der Konfigurationsdatei des Servers eingerichtet.
          Anleitung: docs/home-assistant.md
        </p>
      </>
    );
  } else {
    content = <MqttControls status={status} />;
  }

  return (
    <Section title="Home Assistant">
      <Card>{content}</Card>
    </Section>
  );
}

// The state of the connection, the select "Liste in Home Assistant" and
// "Liste neu senden", which asks the server to send the shopping list
// again (docs/plan.md, F30).
function MqttControls({ status }: { status: MqttStatus }) {
  const queryClient = useQueryClient();
  const setTarget = useMutation(shoppingTargetMutation(queryClient));
  const snapshot = useMutation(shoppingSnapshotMutation);
  const selectId = useId();

  // Hides the notice after sending after a short time; a failure stays
  // until the next tap (ADR-0016).
  const { isSuccess: sent, reset: resetSnapshot } = snapshot;
  useEffect(() => {
    if (!sent) {
      return;
    }
    const timer = setTimeout(resetSnapshot, noticeDuration);
    return () => clearTimeout(timer);
  }, [sent, resetSnapshot]);

  const choice = targetChoice(status);
  // While a choice is saved, the select shows it instead of the stored one.
  const selected = setTarget.isPending
    ? (setTarget.variables ?? noTarget)
    : choice.selected;

  let targetError = "";
  if (setTarget.isError) {
    targetError =
      problemCode(setTarget.error) === "unknown_target"
        ? "Liste wird nicht mehr angeboten"
        : "Speichern fehlgeschlagen";
  }

  let sendNotice = "";
  if (snapshot.isSuccess) {
    sendNotice = "Wird gesendet";
  } else if (snapshot.isError) {
    sendNotice = "Senden fehlgeschlagen";
  }

  return (
    <>
      <p className="text-ink-secondary">{mqttStateText(status.status)}</p>
      <label
        htmlFor={selectId}
        className="mt-3 block text-sm font-medium text-ink-secondary"
      >
        Liste in Home Assistant
      </label>
      <select
        id={selectId}
        value={selected}
        disabled={setTarget.isPending}
        onChange={(event) =>
          setTarget.mutate(event.target.value === noTarget ? null : event.target.value)
        }
        className="mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-2 text-base disabled:opacity-40"
      >
        {choice.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <p role="status" className="mt-1 text-sm font-medium text-danger">
        {targetError}
      </p>
      <button
        type="button"
        disabled={snapshot.isPending}
        onClick={() => snapshot.mutate()}
        className={`mt-3 w-full ${secondaryButtonClass}`}
      >
        Liste neu senden
      </button>
      <p
        role="status"
        className={`mt-1 text-sm font-medium ${snapshot.isError ? "text-danger" : "text-accent"}`}
      >
        {sendNotice}
      </p>
    </>
  );
}

/**
 * The section "Sicherung": the newest of the regular backups, how many
 * there are of how many the server keeps, and the download of a fresh
 * backup with the database and the images.
 */
function BackupSection() {
  const system = useQuery(systemStatusQuery);
  const status = system.data;

  return (
    <Section title="Sicherung">
      {status === undefined ? (
        <Card>
          <LoadState
            failed={system.isError && !system.isFetching}
            onRetry={() => void system.refetch()}
          />
        </Card>
      ) : (
        <Rows
          rows={[
            ["Letzte Sicherung", lastBackupText(status.last_backup_at)],
            ["Aufbewahrt", backupCountText(status.backup_count, status.backup_keep)],
          ]}
        />
      )}
      <a
        href={backupUrl}
        download
        className={`mt-3 flex w-full items-center justify-center text-center ${secondaryButtonClass}`}
      >
        Sicherung herunterladen
      </a>
    </Section>
  );
}

/**
 * The section "Über StashBert": the version, the size of the database,
 * whether Open Food Facts is used, and the source of the product data.
 */
function AboutSection() {
  const system = useQuery(systemStatusQuery);
  const status = system.data;

  return (
    <Section title="Über StashBert">
      {status === undefined ? (
        <Card>
          <LoadState
            failed={system.isError && !system.isFetching}
            onRetry={() => void system.refetch()}
          />
        </Card>
      ) : (
        <Rows
          rows={[
            ["Version", status.version],
            ["Datenbank", formatSize(status.database_size)],
            ["Open Food Facts", openFoodFactsText(status.open_food_facts)],
          ]}
        />
      )}
      <p className="mt-2 px-4 text-sm text-ink-tertiary">
        Produktdaten und Bilder: Open Food Facts (ODbL)
      </p>
    </Section>
  );
}
