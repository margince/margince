import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import { historySubject } from "../records/historyline";

type PhaseMover = components["schemas"]["PrincipalAttribution"];

/**
 * Who made a phase move, as a sentence. The subject comes from the helper the
 * record History uses, so an agent reads "<colleague>, via an agent," here too.
 * A move with no recorded actor, or an actor kind this client does not know,
 * says so; it is never read as the system.
 */
export function phaseMoverSentence(
  who: PhaseMover | null | undefined,
  opened: boolean,
  t: Translator,
): string {
  const subject = who ? historySubject(who, t) : null;
  if (subject === null) return t("project.history.unknown");
  return t(opened ? "project.history.openedBy" : "project.history.movedBy", {
    subject,
  });
}
