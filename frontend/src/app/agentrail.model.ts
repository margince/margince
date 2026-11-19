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
  // The model that answered, not the one that indexes search or a call that
  // failed: an embedding is named only when it is all there is.
  const answering = read.calls.find(
    (call) => call.kind !== "embedding" && !call.error_sentinel,
  );
  if (answering) {
    return `${answering.provider}/${answering.served_model}`;
  }
  // Every recent answer failing is the state the user most needs to see, so
  // it outranks the search index. A call that never reached a model has no
  // served model, so it names the configured one.
  const failed = read.calls.find((call) => call.kind !== "embedding");
  if (failed) {
    return t("agent.fact.noAnswers", {
      model: `${failed.provider}/${failed.served_model || failed.model_id}`,
    });
  }
  const indexing = read.calls.find((call) => call.kind === "embedding");
  if (indexing) {
    return t("agent.fact.searchIndex", {
      model: `${indexing.provider}/${indexing.served_model}`,
    });
  }
  return t(read.allowed ? "agent.fact.noCalls" : "agent.fact.hidden");
}
