import { problemCode } from "./api/client";
import type { components } from "./api/schema";
import { recognizedFields, type RecognitionResult } from "./productForm";

/**
 * The settings of the product recognition (ADR-0021): provider, model,
 * whether a key is stored and its last four characters, and the default
 * model of each provider. The key itself never leaves the server.
 */
export type RecognitionSettings = components["schemas"]["RecognitionSettings"];

/** The body of PUT /integrations/recognition. */
export type RecognitionSettingsUpdate = components["schemas"]["RecognitionSettingsUpdate"];

/** A provider of the product recognition. */
export type RecognitionProvider = components["schemas"]["RecognitionProvider"];

/** The choice in the select "Anbieter": a provider, or off. */
export type ProviderChoice = RecognitionProvider | "off";

/** The options of the select "Anbieter" in their order. */
export const providerChoices: readonly ProviderChoice[] = [
  "off",
  "openai",
  "gemini",
  "anthropic",
];

const providerLabels: Record<ProviderChoice, string> = {
  off: "Aus",
  openai: "OpenAI",
  gemini: "Google Gemini",
  anthropic: "Claude",
};

/** Returns the name of choice in the select "Anbieter". */
export function providerLabel(choice: ProviderChoice): string {
  return providerLabels[choice];
}

/** Returns whether value is one of the providerChoices. */
export function isProviderChoice(value: string): value is ProviderChoice {
  return (providerChoices as readonly string[]).includes(value);
}

/** The values of the recognition form as its fields hold them. */
export interface RecognitionForm {
  choice: ProviderChoice;
  /** The typed API key; it only lives here and in the password field. */
  apiKey: string;
  /** The typed model; empty means the default model of the provider. */
  model: string;
}

/**
 * Returns the form for settings: its provider (off without one), no key
 * and its model ("" for the default model).
 */
export function toRecognitionForm(settings: RecognitionSettings): RecognitionForm {
  return {
    choice: settings.provider ?? "off",
    apiKey: "",
    model: settings.model ?? "",
  };
}

/**
 * Returns form with another choice in the select "Anbieter". The field
 * "Modell" gets the stored model if choice is the stored provider, and is
 * empty (the default model) otherwise, because a model belongs to one
 * provider. A typed key stays.
 */
export function withChoice(
  form: RecognitionForm,
  settings: RecognitionSettings,
  choice: ProviderChoice,
): RecognitionForm {
  const model = choice === settings.provider ? (settings.model ?? "") : "";
  return { ...form, choice, model };
}

/** Returns whether a key is stored for choice, the stored provider. */
function keyStored(settings: RecognitionSettings, choice: ProviderChoice): boolean {
  return choice !== "off" && choice === settings.provider && settings.key_set;
}

/**
 * Returns the line above the field "API-Schlüssel" when a key is stored
 * for choice, like "Gespeichert, endet auf …abcd" (docs/plan.md, F34), or
 * "" otherwise. The field may stay empty then.
 */
export function keyHintText(settings: RecognitionSettings, choice: ProviderChoice): string {
  if (!keyStored(settings, choice)) {
    return "";
  }
  return `Gespeichert, endet auf …${settings.key_hint ?? ""}`;
}

/**
 * Returns the default model of choice, the placeholder of the field
 * "Modell", or "" for off.
 */
export function defaultModel(settings: RecognitionSettings, choice: ProviderChoice): string {
  return choice === "off" ? "" : settings.default_models[choice];
}

/**
 * Returns whether saving form needs a key it does not have: a provider is
 * chosen, the field "API-Schlüssel" is empty, and no key is stored for
 * that provider. The server keeps a stored key only while the provider
 * stays the same (architecture.md, 6.2).
 */
export function keyMissing(settings: RecognitionSettings, form: RecognitionForm): boolean {
  return form.choice !== "off" && form.apiKey.trim() === "" && !keyStored(settings, form.choice);
}

/**
 * Returns the body of PUT /integrations/recognition for form: the chosen
 * provider (null for off), with key and model trimmed and left out when
 * empty. Without a key the server keeps the stored one; without a model
 * it uses the default model.
 */
export function recognitionUpdate(form: RecognitionForm): RecognitionSettingsUpdate {
  if (form.choice === "off") {
    return { provider: null };
  }
  const update: RecognitionSettingsUpdate = { provider: form.choice };
  const model = form.model.trim();
  if (model !== "") {
    update.model = model;
  }
  const apiKey = form.apiKey.trim();
  if (apiKey !== "") {
    update.api_key = apiKey;
  }
  return update;
}

/** The message when "Speichern" needs a key that is not typed in. */
export const keyMissingText = "Bitte einen API-Schlüssel eingeben.";

/**
 * Returns the message for settings the server did not save, by the code of
 * its Problem Details (architecture.md, 6.4), or "Speichern fehlgeschlagen"
 * for every other failure, a network error included. invalid_request only
 * comes from a new provider without a key.
 */
export function saveErrorText(error: unknown): string {
  switch (problemCode(error)) {
    case "invalid_api_key":
      return "Der Schlüssel wird nicht angenommen.";
    case "recognition_failed":
      return "Der Anbieter ist gerade nicht erreichbar.";
    case "invalid_request":
      return keyMissingText;
    default:
      return "Speichern fehlgeschlagen";
  }
}

/**
 * Returns whether the field "API-Schlüssel" is marked as invalid: the tap
 * on "Speichern" was stopped for a missing key (blocked), the provider did
 * not take the key, or the server asked for one.
 */
export function keyRefused(error: unknown, blocked: boolean): boolean {
  const code = problemCode(error);
  return blocked || code === "invalid_api_key" || code === "invalid_request";
}

/** The state of saving the settings, like the status of a mutation. */
export type SaveStatus = "idle" | "pending" | "error" | "success";

/** The message below "Speichern". */
export interface SaveNotice {
  text: string;
  tone: "progress" | "success" | "failure";
}

/**
 * Returns the message below "Speichern" (docs/plan.md, F34): "Bitte einen
 * API-Schlüssel eingeben." when the tap was stopped for a missing key
 * (blocked), otherwise nothing before the first try, "Wird geprüft …"
 * while the server checks the key with the provider, then "Gespeichert"
 * or the message for error.
 */
export function saveNotice(status: SaveStatus, error: unknown, blocked: boolean): SaveNotice {
  if (blocked) {
    return { text: keyMissingText, tone: "failure" };
  }
  switch (status) {
    case "idle":
      return { text: "", tone: "progress" };
    case "pending":
      return { text: "Wird geprüft …", tone: "progress" };
    case "error":
      return { text: saveErrorText(error), tone: "failure" };
    case "success":
      return { text: "Gespeichert", tone: "success" };
  }
}

/** The title of the question before the recognition is switched off. */
export const turnOffTitle = "Produkterkennung ausschalten?";

/** The text of the question before the recognition is switched off. */
export const turnOffQuestion = "Der API-Schlüssel wird gelöscht.";

/** The message when switching the recognition off failed. */
export const turnOffErrorText = "Ausschalten fehlgeschlagen";

/**
 * Returns the note below the section "Produkterkennung" (docs/plan.md,
 * F34): where the photos go and when costs arise, and for Gemini that
 * Google may use the input of its free tier.
 */
export function recognitionNote(choice: ProviderChoice): string {
  const note =
    "Fotos werden an OpenAI, Google bzw. Anthropic geschickt. Kosten entstehen nur, " +
    "wenn du auf der Produktseite „Mit KI erkennen“ antippst.";
  if (choice !== "gemini") {
    return note;
  }
  return `${note} Im kostenlosen Zugang darf Google die Eingaben zur Verbesserung seiner Produkte nutzen.`;
}

/**
 * Returns whether the product page offers "Mit KI erkennen" (docs/plan.md,
 * F35): a provider is set up and its key is stored. Without settings (not
 * loaded yet or failed) it does not.
 */
export function recognitionReady(settings: RecognitionSettings | undefined): boolean {
  return settings !== undefined && settings.provider !== null && settings.key_set;
}

/**
 * The hint below "Mit KI erkennen" while the product has no photo, and the
 * message for no_image.
 */
export const noImageText = "Erst ein Foto aufnehmen";

/**
 * Returns the message for a recognition that failed, by the code of its
 * Problem Details (architecture.md, 6.4), or "Erkennung fehlgeschlagen"
 * for every other failure, a network error or a timeout included.
 */
export function recognizeErrorText(error: unknown): string {
  switch (problemCode(error)) {
    case "no_image":
      return noImageText;
    case "recognition_disabled":
      return "Produkterkennung ist nicht eingerichtet";
    case "invalid_api_key":
      return "Der API-Schlüssel wird nicht angenommen";
    default:
      return "Erkennung fehlgeschlagen";
  }
}

/** The state of a recognition, like the status of a mutation. */
export type RecognizeStatus = "idle" | "pending" | "error" | "success";

/** The message below the buttons of the image about the last recognition. */
export interface RecognizeNotice {
  text: string;
  tone: "neutral" | "success" | "failure";
}

/**
 * Returns the message about the last recognition (docs/plan.md, F35):
 * nothing before the first one and while it runs (the button says "Wird
 * erkannt …" then), "Vorschlag eingetragen. Bitte prüfen und speichern."
 * when result has a readable field, "Auf dem Foto war nichts lesbar."
 * when it has none, or the message for error.
 */
export function recognizeNotice(
  status: RecognizeStatus,
  result: RecognitionResult | undefined,
  error: unknown,
): RecognizeNotice {
  switch (status) {
    case "idle":
    case "pending":
      return { text: "", tone: "neutral" };
    case "error":
      return { text: recognizeErrorText(error), tone: "failure" };
    case "success":
      if (result !== undefined && Object.keys(recognizedFields(result)).length > 0) {
        return { text: "Vorschlag eingetragen. Bitte prüfen und speichern.", tone: "success" };
      }
      return { text: "Auf dem Foto war nichts lesbar.", tone: "neutral" };
  }
}
