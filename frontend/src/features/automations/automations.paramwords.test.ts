// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { TEMPLATE_WORDS } from "./automations.catalogwords";
import { AUTOMATION_CATALOG } from "./automations.fixtures";
import { paramFields, paramsFromValues } from "./automations.params";
import { missingParam } from "./automations.paramwords";

// The backend gates ask which keys need words, and the catalog tests that none
// is empty. This asks that a description is translated, not copied.
describe("the automation catalog's words", () => {
  it.each(Object.entries(TEMPLATE_WORDS))(
    "describes %s in each language's own words",
    (_, words) => {
      expect(de[words.description]).not.toBe(en[words.description]);
      expect(vi[words.description]).not.toBe(en[words.description]);
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
