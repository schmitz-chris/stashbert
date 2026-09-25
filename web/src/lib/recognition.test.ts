import { describe, expect, it } from "vitest";
import {
  defaultModel,
  isProviderChoice,
  keyHintText,
  keyMissing,
  keyMissingText,
  keyRefused,
  providerChoices,
  providerLabel,
  recognitionNote,
  recognitionUpdate,
  saveErrorText,
  saveNotice,
  toRecognitionForm,
  turnOffErrorText,
  turnOffQuestion,
  turnOffTitle,
  withChoice,
  type RecognitionForm,
  type RecognitionSettings,
} from "./recognition";

const defaultModels = {
  openai: "gpt-6-luna",
  gemini: "gemini-3.5-flash-lite",
  anthropic: "claude-sonnet-5",
};

// The recognition switched off.
const off: RecognitionSettings = {
  provider: null,
  model: null,
  key_set: false,
  key_hint: null,
  default_models: defaultModels,
};

// OpenAI with a stored key ending in "abcd" and the default model.
const openai: RecognitionSettings = {
  provider: "openai",
  model: null,
  key_set: true,
  key_hint: "abcd",
  default_models: defaultModels,
};

// Gemini with a stored key and a model of its own.
const gemini: RecognitionSettings = {
  provider: "gemini",
  model: "gemini-3.5-pro",
  key_set: true,
  key_hint: "x9Z1",
  default_models: defaultModels,
};

function form(fields: Partial<RecognitionForm>): RecognitionForm {
  return { choice: "off", apiKey: "", model: "", ...fields };
}

// Problem Details as the client throws them.
function problem(code: string) {
  return { title: "Fehler", status: 400, code };
}

describe("providerLabel", () => {
  it.each([
    ["off", "Aus"],
    ["openai", "OpenAI"],
    ["gemini", "Google Gemini"],
    ["anthropic", "Claude"],
  ] as const)("names %s as %s", (choice, label) => {
    expect(providerLabel(choice)).toBe(label);
  });

  it("offers Aus first, then OpenAI, Google Gemini and Claude", () => {
    expect(providerChoices.map(providerLabel)).toEqual([
      "Aus",
      "OpenAI",
      "Google Gemini",
      "Claude",
    ]);
  });
});

describe("isProviderChoice", () => {
  it.each(["off", "openai", "gemini", "anthropic"])("takes %s", (value) => {
    expect(isProviderChoice(value)).toBe(true);
  });

  it.each(["", "Aus", "claude", "OpenAI"])("refuses %j", (value) => {
    expect(isProviderChoice(value)).toBe(false);
  });
});

describe("toRecognitionForm", () => {
  it("starts with Aus, no key and no model without a provider", () => {
    expect(toRecognitionForm(off)).toEqual({ choice: "off", apiKey: "", model: "" });
  });

  it("starts with the stored provider and an empty key field", () => {
    expect(toRecognitionForm(openai)).toEqual({ choice: "openai", apiKey: "", model: "" });
  });

  it("starts with the stored model", () => {
    expect(toRecognitionForm(gemini)).toEqual({
      choice: "gemini",
      apiKey: "",
      model: "gemini-3.5-pro",
    });
  });
});

describe("withChoice", () => {
  it("empties the model for another provider and keeps the typed key", () => {
    const current = form({ choice: "gemini", apiKey: "sk-neu", model: "gemini-3.5-pro" });
    expect(withChoice(current, gemini, "anthropic")).toEqual({
      choice: "anthropic",
      apiKey: "sk-neu",
      model: "",
    });
  });

  it("brings back the stored model for the stored provider", () => {
    const current = form({ choice: "openai", model: "" });
    expect(withChoice(current, gemini, "gemini")).toEqual({
      choice: "gemini",
      apiKey: "",
      model: "gemini-3.5-pro",
    });
  });

  it("empties the model for Aus", () => {
    const current = form({ choice: "gemini", model: "gemini-3.5-pro" });
    expect(withChoice(current, gemini, "off").model).toBe("");
  });
});

describe("keyHintText", () => {
  it("names the last four characters of the stored key", () => {
    expect(keyHintText(openai, "openai")).toBe("Gespeichert, endet auf …abcd");
  });

  it("says nothing for another provider than the stored one", () => {
    expect(keyHintText(openai, "gemini")).toBe("");
    expect(keyHintText(openai, "off")).toBe("");
  });

  it("says nothing without a stored key", () => {
    expect(keyHintText(off, "openai")).toBe("");
  });
});

describe("defaultModel", () => {
  it.each([
    ["openai", "gpt-6-luna"],
    ["gemini", "gemini-3.5-flash-lite"],
    ["anthropic", "claude-sonnet-5"],
  ] as const)("returns the default model of %s from the server", (choice, model) => {
    expect(defaultModel(off, choice)).toBe(model);
  });

  it("returns nothing for Aus", () => {
    expect(defaultModel(off, "off")).toBe("");
  });
});

describe("keyMissing", () => {
  it("lets the field stay empty for the stored provider with its key", () => {
    expect(keyMissing(openai, form({ choice: "openai" }))).toBe(false);
  });

  it("asks for a key for another provider", () => {
    expect(keyMissing(openai, form({ choice: "anthropic" }))).toBe(true);
  });

  it("asks for a key when none is stored", () => {
    expect(keyMissing(off, form({ choice: "openai" }))).toBe(true);
  });

  it("counts a key of blanks as missing", () => {
    expect(keyMissing(off, form({ choice: "gemini", apiKey: "   " }))).toBe(true);
  });

  it("is content with a typed key", () => {
    expect(keyMissing(openai, form({ choice: "gemini", apiKey: "AIza-neu" }))).toBe(false);
  });

  it("needs no key for Aus", () => {
    expect(keyMissing(off, form({ choice: "off" }))).toBe(false);
  });
});

describe("recognitionUpdate", () => {
  it("sends the provider with the trimmed key and model", () => {
    expect(
      recognitionUpdate(form({ choice: "anthropic", apiKey: " sk-ant-1234 ", model: " claude-x " })),
    ).toEqual({ provider: "anthropic", api_key: "sk-ant-1234", model: "claude-x" });
  });

  it("leaves out an empty key, so the server keeps the stored one", () => {
    expect(recognitionUpdate(form({ choice: "openai", model: "gpt-6" }))).toEqual({
      provider: "openai",
      model: "gpt-6",
    });
  });

  it("leaves out an empty model, so the default model applies", () => {
    expect(recognitionUpdate(form({ choice: "gemini", apiKey: "AIza", model: "  " }))).toEqual({
      provider: "gemini",
      api_key: "AIza",
    });
  });

  it("switches off with provider null only", () => {
    expect(recognitionUpdate(form({ choice: "off", apiKey: "sk", model: "m" }))).toEqual({
      provider: null,
    });
  });
});

describe("saveErrorText", () => {
  it.each([
    ["invalid_api_key", "Der Schlüssel wird nicht angenommen."],
    ["recognition_failed", "Der Anbieter ist gerade nicht erreichbar."],
    ["invalid_request", "Bitte einen API-Schlüssel eingeben."],
    ["internal", "Speichern fehlgeschlagen"],
  ])("names the code %s as %s", (code, text) => {
    expect(saveErrorText(problem(code))).toBe(text);
  });

  it("says Speichern fehlgeschlagen for a network error", () => {
    expect(saveErrorText(new TypeError("Load failed"))).toBe("Speichern fehlgeschlagen");
  });
});

describe("keyRefused", () => {
  it("marks the key field when the provider refused the key", () => {
    expect(keyRefused(problem("invalid_api_key"), false)).toBe(true);
  });

  it("marks the key field when the server asked for a key", () => {
    expect(keyRefused(problem("invalid_request"), false)).toBe(true);
  });

  it("marks the key field when the tap was stopped for a missing key", () => {
    expect(keyRefused(null, true)).toBe(true);
  });

  it("leaves the key field alone when the provider cannot be reached", () => {
    expect(keyRefused(problem("recognition_failed"), false)).toBe(false);
    expect(keyRefused(null, false)).toBe(false);
  });
});

describe("saveNotice", () => {
  it("says nothing before the first try", () => {
    expect(saveNotice("idle", null, false)).toEqual({ text: "", tone: "progress" });
  });

  it("says Wird geprüft while the server checks the key", () => {
    expect(saveNotice("pending", null, false)).toEqual({
      text: "Wird geprüft …",
      tone: "progress",
    });
  });

  it("says Gespeichert after saving", () => {
    expect(saveNotice("success", null, false)).toEqual({ text: "Gespeichert", tone: "success" });
  });

  it("names a refused key", () => {
    expect(saveNotice("error", problem("invalid_api_key"), false)).toEqual({
      text: "Der Schlüssel wird nicht angenommen.",
      tone: "failure",
    });
  });

  it("asks for a key when the tap was stopped for a missing one", () => {
    expect(saveNotice("idle", null, true)).toEqual({ text: keyMissingText, tone: "failure" });
    expect(saveNotice("success", null, true)).toEqual({ text: keyMissingText, tone: "failure" });
  });
});

describe("the question before switching off", () => {
  it("says that the key is deleted", () => {
    expect(turnOffTitle).toBe("Produkterkennung ausschalten?");
    expect(turnOffQuestion).toBe("Der API-Schlüssel wird gelöscht.");
    expect(turnOffErrorText).toBe("Ausschalten fehlgeschlagen");
  });
});

describe("recognitionNote", () => {
  const note =
    "Fotos werden an OpenAI, Google bzw. Anthropic geschickt. Kosten entstehen nur, " +
    "wenn du auf der Produktseite „Mit KI erkennen“ antippst.";
  const geminiNote =
    "Im kostenlosen Zugang darf Google die Eingaben zur Verbesserung seiner Produkte nutzen.";

  it.each(["off", "openai", "anthropic"] as const)(
    "says where the photos go and when costs arise for %s",
    (choice) => {
      expect(recognitionNote(choice)).toBe(note);
    },
  );

  it("adds the use of the free tier by Google for Gemini", () => {
    expect(recognitionNote("gemini")).toBe(`${note} ${geminiNote}`);
  });
});
