import type { KeyboardEvent } from "react";

export const FIELD_COMMIT_KEY = "Enter";

/** A paragraph commits on ⌘/Ctrl+Enter, because Enter there is a newline. */
export function isParagraphCommit(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey">,
): boolean {
  return event.key === FIELD_COMMIT_KEY && (event.metaKey || event.ctrlKey);
}
