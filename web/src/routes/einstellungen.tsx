import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type ChangeEvent, type ReactNode } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { ConfirmActions, Dialog } from "../components/Dialog";
import { PageHeading } from "../components/PageHeading";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { problemCode } from "../lib/api/client";
import {
  backupRestoreMutation,
  healthAnswers,
  mqttStatusQuery,
  recognitionSettingsMutation,
  recognitionSettingsQuery,
  shoppingSnapshotMutation,
  shoppingTargetMutation,
  systemStatusQuery,
} from "../lib/api/queries";
import { pageTitle } from "../lib/pageTitle";
import {
  defaultModel,
  isProviderChoice,
  keyHintText,
  keyMissing,
  keyRefused,
  providerChoices,
  providerLabel,
  recognitionNote,
  recognitionUpdate,
  saveNotice,
  toRecognitionForm,
  turnOffErrorText,
  turnOffQuestion,
  turnOffTitle,
  withChoice,
  type RecognitionForm,
  type RecognitionSettings,
} from "../lib/recognition";
import {
  backupCountText,
  formatSize,
  lastBackupText,
  mqttStateText,
  openFoodFactsText,
  restoreNotice,
  restoreQuestionText,
  waitForRestart,
  type RestartStatus,
  type RestoreNotice,
} from "../lib/settings";
import { noTarget, targetChoice, type MqttStatus } from "../lib/shoppingTarget";

// How long the notices after sending the list and after saving stay
// visible. Failures stay until the next action (ADR-0016).
const noticeDuration = 2000;

// Bordered buttons with the color of their text left open.
const borderedButtonClass =
  "pressable min-h-11 rounded-lg border border-line-strong bg-surface px-4 py-2 font-medium disabled:opacity-40";
// Secondary buttons (docs/plan.md, F22).
const secondaryButtonClass = `${borderedButtonClass} text-accent`;
// "Speichern" of the product recognition, the one filled button of the page
// (docs/plan.md, F22).
const primaryButtonClass =
  "pressable min-h-11 rounded-lg bg-accent px-4 py-2 font-medium text-white disabled:opacity-40";

const labelClass = "block text-sm font-medium text-ink-secondary";
const inputClass =
  "mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 py-2 text-base aria-[invalid=true]:border-danger";

// The download of a fresh backup. A download is navigation, not an API
// call, so it is the one address of the API outside the generated client
// (docs/plan.md, F32).
const backupUrl = "/api/v1/backup";

// The files offered for "Backup einspielen": the archives of "Backup
// herunterladen" (stashbert-<time>.tar.gz), by extension and by type.
const backupFileTypes = ".gz,.tar.gz,application/gzip,application/x-gzip";

// The color of the notices about restoring a backup and about saving the
// product recognition.
const noticeToneClass: Record<RestoreNotice["tone"], string> = {
  progress: "text-ink-secondary",
  success: "text-accent",
  failure: "text-danger",
};

/**
 * The settings (docs/plan.md, F32), opened by the gear in the stock view:
 * Home Assistant, the backups, the product recognition and information
 * about StashBert. Connection data stays in the configuration file of the
 * server; the page only shows whether it is set. The one exception is the
 * key of the product recognition, which is set here but never shown
 * (architecture.md, 4.2).
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
      <RecognitionSection />
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
 * The section "Backup": the newest of the regular backups, how many there
 * are of how many the server keeps, the download of a fresh backup with
 * the database and the images, and restoring such a backup
 * (docs/plan.md, F32 and F33).
 */
function BackupSection() {
  const system = useQuery(systemStatusQuery);
  const status = system.data;
  const queryClient = useQueryClient();
  const restore = useMutation(backupRestoreMutation);
  // The picked backup; the question before restoring it shows while set.
  const [file, setFile] = useState<File | null>(null);
  // After the upload: whether StashBert is back after its restart.
  const [restart, setRestart] = useState<RestartStatus>("waiting");
  const inputRef = useRef<HTMLInputElement>(null);

  // After the answer 202 StashBert restarts. Until it answers again, it is
  // asked every 500 ms for at most 60 s; then all data is loaded again.
  const waiting = restore.isSuccess && restart === "waiting";
  useEffect(() => {
    if (!waiting) {
      return;
    }
    const controller = new AbortController();
    void waitForRestart(healthAnswers, controller.signal).then((answered) => {
      if (controller.signal.aborted) {
        return;
      }
      if (answered) {
        void queryClient.invalidateQueries();
      }
      setRestart(answered ? "answered" : "silent");
    });
    return () => controller.abort();
  }, [waiting, queryClient]);

  // The buttons are locked from the upload until StashBert is back.
  const running = restore.isPending || waiting;
  const notice = restoreNotice(restore.status, restore.error, restart);

  function pickFile(event: ChangeEvent<HTMLInputElement>) {
    const picked = event.target.files?.[0];
    // Lets the same file be picked again next time.
    event.target.value = "";
    if (picked !== undefined) {
      setFile(picked);
    }
  }

  function confirmRestore() {
    if (file === null) {
      return;
    }
    setRestart("waiting");
    restore.mutate(file);
    setFile(null);
  }

  return (
    <Section title="Backup">
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
            ["Letztes Backup", lastBackupText(status.last_backup_at)],
            ["Aufbewahrt", backupCountText(status.backup_count, status.backup_keep)],
          ]}
        />
      )}
      <a
        href={backupUrl}
        download
        aria-disabled={running}
        onClick={(event) => {
          if (running) {
            event.preventDefault();
          }
        }}
        className={`mt-3 flex w-full items-center justify-center text-center aria-disabled:opacity-40 ${secondaryButtonClass}`}
      >
        Backup herunterladen
      </a>
      <button
        type="button"
        disabled={running}
        onClick={() => {
          // A notice of the last try stays until the next action (ADR-0016).
          restore.reset();
          inputRef.current?.click();
        }}
        className={`mt-3 w-full ${secondaryButtonClass}`}
      >
        Backup einspielen
      </button>
      <input
        ref={inputRef}
        type="file"
        accept={backupFileTypes}
        hidden
        onChange={pickFile}
      />
      <p role="status" className={`mt-1 text-sm font-medium ${noticeToneClass[notice.tone]}`}>
        {notice.text}
      </p>
      <Dialog open={file !== null} onClose={() => setFile(null)} title="Backup einspielen?">
        <p className="mt-2 break-words text-ink-secondary">
          {restoreQuestionText(file?.name ?? "")}
        </p>
        <ConfirmActions
          label="Einspielen"
          pending={false}
          onCancel={() => setFile(null)}
          onConfirm={confirmRestore}
        />
      </Dialog>
    </Section>
  );
}

/**
 * The section "Produkterkennung" (docs/plan.md, F34, ADR-0021): provider,
 * API key and model of the recognition of products from their photo, with
 * "Speichern", "Ausschalten" and a note where the photos go.
 */
function RecognitionSection() {
  const recognition = useQuery(recognitionSettingsQuery);
  const settings = recognition.data;

  return (
    <Section title="Produkterkennung">
      {settings === undefined ? (
        <>
          <Card>
            <LoadState
              failed={recognition.isError && !recognition.isFetching}
              onRetry={() => void recognition.refetch()}
            />
          </Card>
          <p className="mt-2 px-4 text-sm text-ink-tertiary">{recognitionNote("off")}</p>
        </>
      ) : (
        <RecognitionControls settings={settings} />
      )}
    </Section>
  );
}

// The form of the product recognition for the stored settings. The typed
// key lives only in the state of the form and in the password field; it
// is never shown elsewhere and never stored in the browser.
function RecognitionControls({ settings }: { settings: RecognitionSettings }) {
  const queryClient = useQueryClient();
  const save = useMutation(recognitionSettingsMutation(queryClient));
  const turnOff = useMutation(recognitionSettingsMutation(queryClient));
  const [form, setForm] = useState<RecognitionForm>(() => toRecognitionForm(settings));
  // Whether the last tap on "Speichern" was stopped for a missing key.
  const [blocked, setBlocked] = useState(false);
  // Whether the question before switching off is shown.
  const [asking, setAsking] = useState(false);
  const providerId = useId();
  const keyId = useId();
  const keyHintId = useId();
  const noticeId = useId();
  const modelId = useId();

  // Hides "Gespeichert" after a short time; a failure stays until the next
  // action (ADR-0016).
  const { isSuccess: saved, reset: resetSave } = save;
  useEffect(() => {
    if (!saved) {
      return;
    }
    const timer = setTimeout(resetSave, noticeDuration);
    return () => clearTimeout(timer);
  }, [saved, resetSave]);

  const keyHint = keyHintText(settings, form.choice);
  const notice = saveNotice(save.status, save.error, blocked);
  const keyInvalid = keyRefused(save.error, blocked);
  // The key field is described by the line about the stored key and by the
  // message that refused the key.
  const keyDescription = [keyHint !== "" && keyHintId, keyInvalid && noticeId]
    .filter(Boolean)
    .join(" ");
  const busy = save.isPending || turnOff.isPending;

  // A change of a field ends the message of the last try (ADR-0016).
  function edit(next: RecognitionForm) {
    setForm(next);
    setBlocked(false);
    if (save.isError) {
      save.reset();
    }
  }

  // Switching off goes only through "Ausschalten" and its question.
  function submit() {
    if (form.choice === "off") {
      return;
    }
    if (keyMissing(settings, form)) {
      setBlocked(true);
      return;
    }
    setBlocked(false);
    save.mutate(recognitionUpdate(form), {
      onSuccess: (stored) => setForm(toRecognitionForm(stored)),
    });
  }

  function confirmTurnOff() {
    turnOff.mutate(
      { provider: null },
      {
        onSuccess: (stored) => {
          setAsking(false);
          setBlocked(false);
          save.reset();
          setForm(toRecognitionForm(stored));
        },
      },
    );
  }

  return (
    <>
      <Card>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <label htmlFor={providerId} className={labelClass}>
            Anbieter
          </label>
          <select
            id={providerId}
            value={form.choice}
            disabled={busy}
            onChange={(event) => {
              const { value } = event.target;
              if (isProviderChoice(value)) {
                edit(withChoice(form, settings, value));
              }
            }}
            className="mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-2 text-base disabled:opacity-40"
          >
            {providerChoices.map((choice) => (
              <option key={choice} value={choice}>
                {providerLabel(choice)}
              </option>
            ))}
          </select>
          {form.choice !== "off" && (
            <>
              <label htmlFor={keyId} className={`mt-4 ${labelClass}`}>
                API-Schlüssel
              </label>
              {keyHint !== "" && (
                <p id={keyHintId} className="mt-1 text-sm break-words text-ink-secondary">
                  {keyHint}
                </p>
              )}
              <input
                id={keyId}
                type="password"
                value={form.apiKey}
                onChange={(event) => edit({ ...form, apiKey: event.target.value })}
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                aria-invalid={keyInvalid}
                aria-describedby={keyDescription || undefined}
                className={inputClass}
              />
              <label htmlFor={modelId} className={`mt-4 ${labelClass}`}>
                Modell
              </label>
              <input
                id={modelId}
                value={form.model}
                placeholder={defaultModel(settings, form.choice)}
                onChange={(event) => edit({ ...form, model: event.target.value })}
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                className={inputClass}
              />
              <button
                type="submit"
                disabled={busy}
                className={`mt-4 w-full ${primaryButtonClass}`}
              >
                Speichern
              </button>
              <p
                id={noticeId}
                role="status"
                className={`mt-1 text-sm font-medium break-words ${noticeToneClass[notice.tone]}`}
              >
                {notice.text}
              </p>
            </>
          )}
        </form>
        {settings.provider !== null && (
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              turnOff.reset();
              setAsking(true);
            }}
            className={`mt-3 w-full text-danger ${borderedButtonClass}`}
          >
            Ausschalten
          </button>
        )}
      </Card>
      <p className="mt-2 px-4 text-sm text-ink-tertiary">{recognitionNote(form.choice)}</p>
      <Dialog open={asking} onClose={() => setAsking(false)} title={turnOffTitle}>
        <p className="mt-2 text-ink-secondary">{turnOffQuestion}</p>
        <p role="status" className="mt-2 text-sm font-medium text-danger">
          {turnOff.isError ? turnOffErrorText : ""}
        </p>
        <ConfirmActions
          label="Ausschalten"
          pending={turnOff.isPending}
          onCancel={() => setAsking(false)}
          onConfirm={confirmTurnOff}
        />
      </Dialog>
    </>
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
