import type { Translator } from "../../i18n";
import type { DisplayField } from "../../lib/approvaldisplay.types";
import { humanizeKind } from "./approvalkind";

// joinedWords reads a list of wire strings as one line, skipping blanks.
function joinedWords(
  list: readonly unknown[],
  word: (code: string) => string,
): string | null {
  const words = list.flatMap((code) =>
    typeof code === "string" && code !== "" ? [word(code)] : [],
  );
  return words.length > 0 ? words.join(", ") : null;
}

/**
 * One declared field against one payload, ready for the card to draw.
 *
 * Returns null for a field the payload does not carry. That is ordinary:
 * `previous_close_date` is absent on a deal that never had one. The card drops
 * such a field rather than printing an empty row.
 */
export function displayValue(
  entry: DisplayField,
  raw: unknown,
  t: Translator,
  formatDay: (value: string) => string,
): string | null {
  if (raw === null || raw === undefined) {
    return null;
  }
  if (entry.as === "enum") {
    // A list or a single code: `flags` is an array, `reason` is one string.
    // An unmapped code degrades to its own words rather than vanishing.
    return joinedWords(Array.isArray(raw) ? raw : [raw], (code) => {
      const key = entry.optionLabels?.[code];
      return key ? t(key) : humanizeKind(code);
    });
  }
  if (entry.as === "text" && Array.isArray(raw)) {
    // A list of names, such as the stages a move skips, reads as one line.
    return joinedWords(raw, (name) => name);
  }
  if (typeof raw !== "string") {
    // Numbers reach the reader as their own digits. Anything structured is left
    // out, because a JSON dump under a label would misstate what it introduces.
    return typeof raw === "number" || typeof raw === "boolean"
      ? String(raw)
      : null;
  }
  if (raw.trim() === "") {
    return null;
  }
  return entry.as === "date" ? formatDay(raw) : raw;
}
