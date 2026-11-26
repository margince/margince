import { isChordOf } from "./chord";

export const FIELD_COMMIT_KEY = "Enter";

/** A paragraph commits on ⌘/Ctrl+Enter, because Enter there is a newline. */
export function isParagraphCommit(
  event: Parameters<typeof isChordOf>[0],
): boolean {
  return isChordOf(event, FIELD_COMMIT_KEY);
}
