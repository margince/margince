import type { components } from "../api/schema";
import type { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type ConsentVerdict =
  components["schemas"]["ContactConsentGuardEntry"]["verdict"];

// The word the rail's consent rows put on a channel's verdict, read from the
// SERVER's key rather than the rendered word: a translated label must not
// change how the slot is coloured.
//
// A table rather than a switch, so a verdict the contract gains and this does
// not fails the BUILD. The `??` below is the same question asked of the WIRE,
// where the type is only a claim: a cached page talking to a newer API is
// handed a verdict this build has no word for, and an unmatched key would put
// an empty cell where a permission belongs.
const VERDICT_WORD: Record<ConsentVerdict, MessageKey> = {
  allowed: "contact.consent.allowedWord",
  blocked: "contact.consent.blockedWord",
  unknown: "contact.consent.unknownWord",
};

export function consentWord(
  verdict: ConsentVerdict | undefined,
  t: ReturnType<typeof useT>,
): string {
  // No entry, and a verdict off a newer server, land on the same word: in both
  // this build has no recorded answer to show.
  return t((verdict && VERDICT_WORD[verdict]) ?? "contact.consent.unknownWord");
}
