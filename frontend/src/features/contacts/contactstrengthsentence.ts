import type { components } from "../../api/schema";
import { formatDayMonth } from "../../format/format.dates";
import type { Locale, Translator } from "../../i18n";

type Strength = components["schemas"]["RelationshipStrength"];

// Recency halves every 30 days, so a third is roughly seven weeks of silence.
const STALE_BELOW = 1 / 3;
// Below a half, one side sent more than five times what the other did.
const LOPSIDED_BELOW = 0.5;

/**
 * One sentence on where the relationship stands. A weak band names the factor
 * that holds the score down, so a recent last contact never reads as "rare".
 * The counts cover only the shared interactions the score folds. A restricted
 * thread or another contact's reply is not in them, so a missing reply reads
 * as "none counted", never "none sent".
 */
export function strengthSentence(
  strength: Strength,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  if (strength.bucket === "none") {
    return t("standing.ring.noInteractions");
  }
  const last = strength.last_interaction
    ? formatDayMonth(strength.last_interaction, locale, zone)
    : "";
  if (strength.bucket !== "weak") {
    return t(`contact.strength.sentence.${strength.bucket}`, { last });
  }
  const key = weakSentence(strength);
  if (key === "contact.strength.sentence.weakStale" && strength.last_engaged) {
    // Recency counts from their part, so the stale sentence dates it.
    return t(key, {
      last: formatDayMonth(strength.last_engaged, locale, zone),
    });
  }
  return t(key, { last });
}

function weakSentence(strength: Strength) {
  const { recency, reciprocity } = strength.factors;
  // With nothing from them, recency is zero for want of a reply, which the
  // direction sentences below name.
  if (recency < STALE_BELOW && strength.last_engaged) {
    return "contact.strength.sentence.weakStale";
  }
  const inbound = strength.inbound_90d;
  const outbound = strength.outbound_90d;
  if (inbound !== undefined && outbound !== undefined) {
    // Zero directed counts do not prove an interaction was undirected: a
    // meeting or another contact's reply also lands here.
    if (inbound === 0 && outbound === 0) {
      return "contact.strength.sentence.weakUndirected";
    }
    if (inbound === 0) {
      return "contact.strength.sentence.weakNoReply";
    }
    if (outbound === 0) {
      return "contact.strength.sentence.weakOnlyThem";
    }
  }
  return reciprocity < LOPSIDED_BELOW
    ? "contact.strength.sentence.weakLopsided"
    : "contact.strength.sentence.weakFew";
}
