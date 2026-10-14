// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { ImportObject, ImportReasonCode } from "./importtypes";

const OBJECTS: readonly ImportObject[] = ["company", "lead", "contact"];

// The plural the server names a type by: the stem of that type's template file.
const OBJECT_OF_PLURAL: Readonly<Record<string, ImportObject>> = {
  companies: "company",
  leads: "lead",
  contacts: "contact",
};

const FIELD_TYPES = [
  "text",
  "number",
  "date",
  "currency",
  "multiselect",
  "picklist",
  "boolean",
] as const;

function objectOf(value: string): ImportObject | undefined {
  return OBJECTS.find((one) => one === value) ?? OBJECT_OF_PLURAL[value];
}

// The server names a type and a field type in English. A reader of another
// language is told them in their own, so the sentence around them stays whole.
function inReadersWords(
  t: Translator,
  params: Readonly<Record<string, string>>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [name, value] of Object.entries(params)) {
    const object = objectOf(value);
    const fieldType = FIELD_TYPES.find((one) => one === value);
    if (object && ["plural", "object", "from", "to"].includes(name)) {
      out[name] = t(`import.object.${object}`);
    } else if (fieldType && name === "type") {
      out[name] = t(`cf.type.${fieldType}`);
    } else {
      out[name] = value;
    }
  }
  return out;
}

/**
 * A reason in the reader's language, from its code and the values the server
 * names. Three cases read as the English sentence the server wrote instead: a
 * report stored before codes existed, a code this bundle has no sentence for,
 * and a value the sentence needs that the server did not send.
 */
export function reasonText(
  t: Translator,
  english: string,
  code: ImportReasonCode | undefined,
  params: Readonly<Record<string, string>> | undefined,
): string {
  if (code === undefined) return english;
  const key = `import.reason.${code}` as const;
  const sentence = t(key, inReadersWords(t, params ?? {}));
  return sentence === key || /\{\w+\}/.test(sentence) ? english : sentence;
}
