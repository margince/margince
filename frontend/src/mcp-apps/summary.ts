// What a view draws when its call left nothing to decide: what the call did.
// A host frames a panel for every call of a tool with a view. A view that drew
// only decisions would leave most of those panels empty.
//
// The words come from the server. Each change carries its field's label, and
// its values carry a name in place of an id. A view has no catalog and no
// network to look either up.

import { badge } from "./badge";
import { el } from "./bridge";
import { avatar, panel, panelBody } from "./parts";
import { asList, asRecord, asText } from "./types";
import { ABSENT, day, money } from "./values";

/** The fields that name a record, best first. */
const NAME_FIELDS = ["full_name", "display_name", "name", "subject", "email"];

const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;
const WHOLE_UUID = new RegExp(`^${UUID.source}$`, "i");

/** isID reports whether a value is a record id, which a reader cannot use. */
export function isID(value: unknown): boolean {
  return typeof value === "string" && WHOLE_UUID.test(value);
}

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

/** One value as the server shows it: kind says how to format text. */
type Shown = { kind: string; text: string; currency: string };

/** One field a call changed or would change, in the server's words. */
export type Change = {
  field: string;
  label: string;
  before: Shown;
  after: Shown;
};

function shownOf(value: unknown): Shown {
  const v = asRecord(value);
  return {
    kind: asText(v.kind),
    text: asText(v.text),
    currency: asText(v.currency),
  };
}

/** changesOf reads the changes a result carries. */
export function changesOf(value: unknown): Change[] {
  return asList(value)
    .map(asRecord)
    .map((c) => ({
      field: asText(c.field),
      label: asText(c.label),
      before: shownOf(c.before),
      after: shownOf(c.after),
    }))
    .filter((c) => c.field !== "" && c.label !== "");
}

/** inWords names a field the server sent no label for by its words. */
export function inWords(name: string): string {
  const words = name
    .replace(/^cf_/, "")
    .replace(/_id$/, "")
    .replaceAll("_", " ")
    .trim();
  return words === "" ? name : words.charAt(0).toUpperCase() + words.slice(1);
}

/** structured renders a list or an object the server could not put in words. */
function structured(text: string): string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return text;
  }
  const parts = Array.isArray(parsed)
    ? parsed.map((part) => (typeof part === "string" ? part : ""))
    : Object.entries(asRecord(parsed))
        .filter(([, part]) => typeof part === "string" && part !== "")
        .map(([key, part]) => `${inWords(key)}: ${String(part)}`);
  const kept = parts.filter((part) => part !== "");
  return kept.length === 0 ? ABSENT : kept.join(", ");
}

/** shown renders one value for a reader. */
export function shown(field: string, value: Shown): string {
  switch (value.kind) {
    case "":
    case "empty":
      return ABSENT;
    case "money":
      return money(Number(value.text), value.currency);
    case "date":
      return day(value.text);
    case "boolean":
      return value.text === "true" ? "Yes" : "No";
    case "structured":
      return structured(value.text);
    case "unavailable_name":
      return "Name not available";
    case "execution":
      return "Set when approved";
  }
  if (/(_at|_date)$/.test(field) && day(value.text) !== ABSENT) {
    return day(value.text);
  }
  return value.text === "" ? ABSENT : value.text;
}

/**
 * changeTable lists changes as label and value rows. A row shows the value
 * after the call, or both values when the reader needs to see both. It answers
 * null when there is nothing to list.
 */
export function changeTable(
  changes: Change[],
  opts: { skip?: string; before: boolean },
): HTMLElement | null {
  const rows = changes.filter((c) => c.field !== opts.skip);
  if (rows.length === 0) return null;
  const table = el("table", "evidence");
  const body = el("tbody");
  for (const change of rows) {
    const after = shown(change.field, change.after);
    const tr = el("tr");
    tr.append(
      el("th", "meta", change.label),
      el(
        "td",
        "name",
        opts.before
          ? `${shown(change.field, change.before)} → ${after}`
          : after,
      ),
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
  return statusPanel(title, undefined, ...lines);
}

/** savedPanel is the card for a write that went through, marked as saved. */
export function savedPanel(
  title: string,
  ...lines: HTMLElement[]
): HTMLElement {
  return statusPanel(title, badge("Saved", "success"), ...lines);
}

function statusPanel(
  title: string,
  action: HTMLElement | undefined,
  ...lines: HTMLElement[]
): HTMLElement {
  const card = panel(
    title,
    action === undefined ? { level: "h1" } : { level: "h1", action },
  );
  const body = panelBody();
  body.append(...lines);
  card.appendChild(body);
  return card;
}

/**
 * refusalPanel says the call changed nothing. A refusal is written for the
 * assistant. One that names a record id tells it what to do next, and only the
 * assistant can turn that into words for the reader.
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
      words === "" || UUID.test(words) ? "Ask the assistant why." : words,
    ),
  );
}

/**
 * recordLine names the record a write landed on: its chip, its name and its
 * kind, as a row of colleagues reads in the app.
 */
export function recordLine(
  name: string,
  id: string,
  noun: string,
): HTMLElement {
  const line = el("div", "record-line");
  line.append(
    avatar(name, id),
    el("p", "name", name),
    badge(noun.charAt(0).toUpperCase() + noun.slice(1)),
  );
  return line;
}
