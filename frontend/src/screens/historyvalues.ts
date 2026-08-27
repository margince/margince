import { formatDateTime, formatMoneyOrAbsent } from "../format/format";
import { type Locale, translate } from "../i18n";

// A history value read the way the rest of the record page reads it.
//
// The audit spine projects every value as a string, including the integer
// minor-unit count money is stored in, an ISO instant, a jsonb array, a bare
// id, and a storage path. Printed as stored, none of those says what it
// MEANS: a deal moving from 2500000 to 4150000 reads as millions beside tiles
// that show the same figure as €25,000.00 — and a value the reader misreads by
// a hundredfold is the one thing that must not sit next to a button that puts
// changes back; `2026-08-26T02:12:07.551698Z` is a moment a reader has to
// parse by hand; `01a03bd4-...` names nothing they recognise; and
// `dataset:margince-demo-database/datasets/v1/logos/akeneo.com.png` buries the
// one part of it — the file — under a path nobody asked to read. Each rule
// below turns one stored shape into the claim it is actually making; anything
// the rules do not recognise is returned exactly as stored, which is the safe
// default for a value this function cannot explain.
//
// Money is checked FIRST and unconditionally: a `*_minor` column is money
// whatever else it happens to look like. The scale is the CURRENCY's and
// never a constant — `formatMoneyOrAbsent` resolves it through
// `format/minorunits.ts`, which mirrors the server's ISO 4217 table, so a
// currency with no minor unit keeps its digits instead of losing two of them.

// A column holding an integer minor-unit amount, by the suffix the contract
// spells it with. A rule rather than a list: a money column added upstream is
// formatted the day it appears, where a list would render the new one raw.
export function isMinorUnitField(field: string): boolean {
  return field.endsWith("_minor");
}

// RFC 3339 with either tail the backend actually emits — fractional seconds
// and/or a zone offset ("...+07:00", "...551698Z"). Anchored at BOTH ends so a
// value that merely CONTAINS a date-shaped run (a path segment, a free-text
// note) is never mistaken for a timestamp; checked before the path rule below
// for the same reason, since a timestamp's offset ("+07:00") is exactly the
// kind of trailing run a looser path pattern could also claim.
const ISO_TIMESTAMP =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})$/;

// RFC 4122, any version/variant — a BARE id, with no scheme in front of it
// (a value like "human:<uuid>" is a principal grammar, not a bare id, and this
// function has no business reading it).
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// A path or URI: at least one "/" separating two non-blank segments. Checked
// after the timestamp rule (its offset can carry digits and colons but never
// a slash) so a value never matches both.
const PATH_LIKE = /^\S*\/\S+$/;

/** A JSON array, or `undefined` when the value does not parse as one. */
function asJsonArray(value: string): unknown[] | undefined {
  // A value that fails to parse, or parses to something other than an array,
  // is simply not a JSON array — a branch this function takes, not an error
  // condition to report. Every other stored shape (plain strings, uuids)
  // reaches JSON.parse too, and throwing here would make it exceptional for
  // the common case instead of the rare one.
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    return undefined;
  }
  return Array.isArray(parsed) ? parsed : undefined;
}

// Currency/locale/zone/name-resolution for turning a stored history value
// into what it means.
export type HistoryValueCtx = Readonly<{
  // The record's ISO currency, which is what gives a minor-unit column its
  // scale. Absent on a record type that holds no money — the formatter says
  // so rather than printing a bare integer under a currency it was never
  // denominated in.
  currency: string | null | undefined;
  locale: Locale;
  zone: string;
  // Resolves a uuid to the name a reader recognises. A caller that has not
  // wired a lookup yet leaves this unset, and the uuid renders untouched
  // rather than half-explained — a SHORTENED uuid is a value the reader can no
  // longer check against the record it came from.
  nameOf?: (id: string) => string | undefined;
}>;

// The rendered value, or null for a value the record does not hold — the diff
// draws its own "created" and "cleared" wording for that and must keep being
// able to tell the two apart from a value that happens to be empty text.
export function historyValue(
  field: string,
  value: string | null | undefined,
  ctx: HistoryValueCtx,
): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (isMinorUnitField(field)) {
    const minor = Number(value);
    // A minor-unit column whose value is not an integer is not an amount this
    // function can scale, and inventing a reading for it would be a worse
    // answer than the one the spine recorded.
    if (!Number.isInteger(minor)) {
      return value;
    }
    return formatMoneyOrAbsent(minor, ctx.currency, ctx.locale);
  }
  if (ISO_TIMESTAMP.test(value)) {
    return formatDateTime(value, ctx.locale, ctx.zone);
  }
  const items = asJsonArray(value);
  if (items !== undefined) {
    return items.length === 0
      ? translate(ctx.locale, "history.emptyList")
      : items.map((item) => String(item)).join(", ");
  }
  if (UUID.test(value)) {
    // Left EXACTLY as stored when the resolver cannot name it — a shortened
    // uuid is a value the reader can no longer check against the record.
    return ctx.nameOf?.(value) ?? value;
  }
  if (PATH_LIKE.test(value)) {
    const segments = value.split("/");
    return segments[segments.length - 1];
  }
  return value;
}
