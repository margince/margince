import type { components } from "../../api/schema";
import { formatDayMonth } from "../../format/format.dates";
import type { Locale, Translator } from "../../i18n";

type Strength = components["schemas"]["RelationshipStrength"];

// Recency halves every 30 days, so a third is roughly seven weeks of silence.
const STALE_BELOW = 1 / 3;

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
  return t(weakSentence(strength), { last });
}

function weakSentence(strength: Strength) {
  const { recency, frequency, reciprocity } = strength.factors;
  if (recency < STALE_BELOW) {
    return "contact.strength.sentence.weakStale";
  }
  const inbound = strength.inbound_90d;
  const outbound = strength.outbound_90d;
  if (inbound !== undefined && outbound !== undefined) {
    // Undirected interactions, such as a meeting with no recorded side, add
    // to frequency but say nothing about who answered whom.
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
  return reciprocity < frequency
    ? "contact.strength.sentence.weakLopsided"
    : "contact.strength.sentence.weakFew";
}
