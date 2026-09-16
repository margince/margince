import type { useT } from "../i18n";

// The word the rail's consent rows put on a channel's verdict.
//
// `consentWord` turns the server's verdict key into the label the rail draws
// beside the channel it governs, read from the key rather than the rendered
// word so a translation cannot change how the slot is coloured.

// The verdict word and its tone are read from the SERVER's verdict key, never
// from the rendered word: a translated label must not change how the slot is
// coloured. Read by the rail's consent rows, which is where the verdict is
// drawn now that the readings row no longer carries it.
export function consentWord(
  verdict: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  switch (verdict) {
    case "allowed":
      return t("contact.consent.allowedWord");
    case "blocked":
      return t("contact.consent.blockedWord");
    default:
      return t("contact.consent.unknownWord");
  }
}
