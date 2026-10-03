import type { Translator } from "../i18n";
import type { AiCall } from "./agentrail-reads";

/**
 * What the model row prints: the model the last call was SERVED by — not the
 * configured one, because a fallback ladder makes those differ exactly when it
 * matters — or the reason there is none.
 */
export function modelText(
  read: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>,
  t: Translator,
): string {
  // The model that answers, not the one that indexes search: an embedding is
  // named only when it is all there is, and then as what it is.
  const answering = read.calls.find((call) => call.kind !== "embedding");
  if (answering) {
    return `${answering.provider}/${answering.served_model}`;
  }
  const indexing = read.calls[0];
  if (indexing) {
    return t("agent.fact.searchIndex", {
      model: `${indexing.provider}/${indexing.served_model}`,
    });
  }
  return t(read.allowed ? "agent.fact.noCalls" : "agent.fact.hidden");
}
