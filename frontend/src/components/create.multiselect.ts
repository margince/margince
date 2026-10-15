// multiselect (e.g. a webhook's subscribed event types): the toggled
// selection is collected as a comma-joined string in the SAME
// `values: Record<string, string>` channel every scalar field already uses —
// JSON encoding preserves arbitrary option labels; existing consumers retain
// their comma-delimited encoding. Every existing single-string field type stays
// untouched. These are the documented mapper a screen's transport uses to
// recover the `string[]` (join before render, split after submit).
const MULTISELECT_DELIMITER = ",";

export function splitMultiselectValue(
  raw: string,
  encoding?: "json",
): string[] {
  if (encoding === "json") {
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) && parsed.every((v) => typeof v === "string")
      ? parsed
      : [];
  }
  return raw.length === 0 ? [] : raw.split(MULTISELECT_DELIMITER);
}

export function joinMultiselectValue(
  selected: string[],
  encoding?: "json",
): string {
  return encoding === "json"
    ? JSON.stringify(selected)
    : selected.join(MULTISELECT_DELIMITER);
}
