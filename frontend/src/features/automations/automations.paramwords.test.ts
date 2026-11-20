// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { TEMPLATE_WORDS } from "./automations.catalogwords";
import { AUTOMATION_CATALOG } from "./automations.fixtures";
import { paramFields, paramsFromValues } from "./automations.params";
import {
  missingParam,
  OPTION_LABELS,
  PARAM_LABELS,
} from "./automations.paramwords";

// The backend gates ask which keys need words. This asks that each word the
// maps name is written in every language.

function wordedInEveryLanguage(key: MessageKey) {
  for (const [lang, catalog] of Object.entries({ en, de, vi })) {
    expect(catalog[key].trim(), `${lang} ${key}`).not.toBe("");
  }
}

describe("the automation catalog's words", () => {
  it.each(Object.entries(TEMPLATE_WORDS))(
    "names and describes %s in every language",
    (_, words) => {
      wordedInEveryLanguage(words.name);
      wordedInEveryLanguage(words.description);
      expect(de[words.description]).not.toBe(en[words.description]);
      expect(vi[words.description]).not.toBe(en[words.description]);
    },
  );

  it.each(Object.entries(PARAM_LABELS))(
    "labels %s in every language",
    (_, key) => wordedInEveryLanguage(key),
  );

  it.each(Object.entries(OPTION_LABELS))(
    "words every option of %s in every language",
    (_, options) => {
      for (const key of Object.values(options)) wordedInEveryLanguage(key);
    },
  );
});

describe("an optional parameter", () => {
  const routing = AUTOMATION_CATALOG.find(
    (entry) => entry.key === "assign_lead_owner",
  );
  const fields = paramFields(routing?.params_schema ?? {});
  const t = (key: MessageKey) => en[key];

  it("is optional when the schema does not require it", () => {
    expect(
      fields.find((field) => field.key === "cap_per_owner")?.required,
    ).toBe(false);
    expect(missingParam(fields, { cap_per_owner: "" }, t)).toBeUndefined();
  });

  it("is left out when empty, and sent when set", () => {
    expect(paramsFromValues(fields, { cap_per_owner: "" })).toEqual({});
    expect(paramsFromValues(fields, { cap_per_owner: "5" })).toEqual({
      cap_per_owner: 5,
    });
  });
});
