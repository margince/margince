// What a view draws when its call left nothing to decide: what the call did.
// A host frames a panel for every call of a tool with a view. A view that drew
// only decisions would leave most of those panels empty.

import { el } from "./bridge";
import { fieldLabel, isID } from "./fieldlabels";
import { panel, panelBody } from "./parts";
import { asText } from "./types";
import { ABSENT, day, money } from "./values";

/** The fields that name a record, best first. */
const NAME_FIELDS = ["full_name", "display_name", "name", "subject", "email"];

const ANY_ID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/** nameField answers the field that names the record, if one does. */
export function nameField(fields: Record<string, unknown>): string | undefined {
  return NAME_FIELDS.find((key) => {
    const value = asText(fields[key]);
    return value !== "" && !isID(value);
  });
}

/** recordName answers the record's own name, or "" when it has none. */
export function recordName(fields: Record<string, unknown>): string {
  const key = nameField(fields);
  return key === undefined ? "" : asText(fields[key]);
}

/** recordNoun names the kind of record: an activity by its own kind, since
 *  "task" says more than "activity". */
export function recordNoun(
  recordType: string,
  fields: Record<string, unknown>,
): string {
  const kind = asText(fields.kind);
  if (recordType === "activity" && /^[a-z]+$/.test(kind)) return kind;
  return recordType === "" ? "record" : recordType.replaceAll("_", " ");
}

/** withArticle puts "a" or "an" before a noun. */
export function withArticle(noun: string): string {
  return /^[aeiou]/.test(noun) ? `an ${noun}` : `a ${noun}`;
}

/**
 * shownValue renders one field's value for a reader, a list or an object as
 * its parts in words. A record id names nothing a reader knows, so it reads as
 * `linked` instead.
 */
function shownValue(
  name: string,
  value: unknown,
  fields: Record<string, unknown>,
  linked: string,
): string {
  if (Array.isArray(value)) {
    const parts = value.map((part) => shownValue(name, part, fields, linked));
    return parts.length === 0 ? ABSENT : parts.join(", ");
  }
  if (typeof value === "object" && value !== null) {
    const parts = Object.entries(value)
      .filter(([, part]) => part !== null && part !== "")
      .map(
        ([key, part]) =>
          `${fieldLabel(key)}: ${shownValue(key, part, fields, linked)}`,
      );
    return parts.length === 0 ? ABSENT : parts.join(", ");
  }
  if (typeof value === "number" && name.endsWith("_minor")) {
    return money(value, fields.currency);
  }
  if (typeof value === "string" && /(_at|_date)$/.test(name)) {
    const shown = day(value);
    if (shown !== ABSENT) return shown;
  }
  return plainValue(value, linked);
}

function plainValue(value: unknown, linked: string): string {
  if (value === null || value === undefined || value === "") return ABSENT;
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (typeof value === "number") return `${value}`;
  return typeof value !== "string" || isID(value) ? linked : value;
}

/**
 * changeTable lists the fields the call sent, each with the value the record
 * now holds. It answers null when the host did not say what the call sent.
 */
export function changeTable(
  sent: Record<string, unknown>,
  fields: Record<string, unknown>,
  opts: { skip?: string; linked: string },
): HTMLElement | null {
  const names = Object.keys(sent).filter((name) => name !== opts.skip);
  if (names.length === 0) return null;
  const table = el("table", "evidence");
  const body = el("tbody");
  for (const name of names) {
    const value = name in fields ? fields[name] : sent[name];
    const tr = el("tr");
    tr.append(
      el("th", "meta", fieldLabel(name)),
      el("td", "name", shownValue(name, value, fields, opts.linked)),
    );
    body.appendChild(tr);
  }
  table.appendChild(body);
  return table;
}

/** summaryPanel is the card for a call that needs nothing from the reader. */
export function summaryPanel(
  title: string,
  ...lines: HTMLElement[]
): HTMLElement {
  const card = panel(title, { level: "h1" });
  const body = panelBody();
  body.append(...lines);
  card.appendChild(body);
  return card;
}

/**
 * refusalPanel says the call changed nothing, and why when the refusal's words
 * are fit for a reader. A refusal written for the assistant names ids and
 * replay steps, and then only the assistant can say what it means.
 */
export function refusalPanel(
  title: string,
  refusal: string | null,
): HTMLElement {
  const words = refusal ?? "";
  return summaryPanel(
    title,
    el(
      "p",
      "intro",
      words === "" || ANY_ID.test(words) ? "Ask the assistant why." : words,
    ),
  );
}
