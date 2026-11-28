// Placeholders name data fields; the user sees only their substituted values.
export function visibleWords(value: string): string {
  return value.replace(/\{[^}]*\}/g, "");
}
