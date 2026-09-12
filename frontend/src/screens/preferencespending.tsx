import { Card } from "../design-system/atoms";
import { useT } from "../i18n";
import { labelOf, type PurposeView } from "./preferences.logic";

/** One choice the server did not simply apply, and why. */
export type ChoiceOutcome = Readonly<{
  purpose_key: string;
  reason: string;
}>;

/**
 * What the page says about choices the server did not simply apply.
 *
 * They ride the server's `refused` list, and reading that list as "these
 * failed" is the mistake this component exists to prevent. Only one of the
 * three outcomes is a refusal the subject caused; the others are a round trip
 * in progress and a fault on our side.
 *
 *   confirmation_sent        — the subscribe was taken and a link is on its
 *                              way. The purpose rows come back UNCHANGED,
 *                              because nothing is granted until that link is
 *                              spent, so a page saying nothing here would show
 *                              somebody their tick vanishing unexplained.
 *   confirmation_unavailable — this installation cannot mail the confirmation.
 *                              About US, so it must not be reported as their
 *                              choice being declined.
 *   cannot_grant             — the subject's own record cannot take a grant,
 *                              which an erasure is the usual cause of.
 *
 * EACH KIND GETS ITS OWN SENTENCE, and an unknown one gets a plain line rather
 * than silence: a code this build does not recognise still means the choice did
 * not take, and showing nothing would leave the subject believing it did.
 */
export function PendingConfirmations({
  outcomes,
  purposes,
}: Readonly<{
  outcomes: readonly ChoiceOutcome[];
  purposes: PurposeView[];
}>) {
  const t = useT();
  if (outcomes.length === 0) return null;
  const named = (reason: string) =>
    outcomes
      .filter((outcome) => outcome.reason === reason)
      .map((outcome) => {
        const purpose = purposes.find(
          (candidate) => candidate.key === outcome.purpose_key,
        );
        // The key itself if the purpose is not in the view, which is a state
        // the server should not produce — better a raw key than a sentence
        // with a blank where a name belongs.
        return purpose ? labelOf(t, purpose) : outcome.purpose_key;
      });
  const known = new Set([
    "confirmation_sent",
    "confirmation_unavailable",
    "cannot_grant",
  ]);
  const unrecognised = outcomes.filter(
    (outcome) => !known.has(outcome.reason),
  ).length;

  const sent = named("confirmation_sent");
  const unavailable = named("confirmation_unavailable");
  const refused = named("cannot_grant");
  return (
    <Card as="div" inset className="pref-partial-banner">
      {sent.length > 0 && (
        <p>{t("prefs.confirmationSent", { purposes: sent.join(", ") })}</p>
      )}
      {unavailable.length > 0 && (
        <p>
          {t("prefs.confirmationUnavailable", {
            purposes: unavailable.join(", "),
          })}
        </p>
      )}
      {refused.length > 0 && (
        <p>{t("prefs.cannotGrant", { purposes: refused.join(", ") })}</p>
      )}
      {unrecognised > 0 && <p>{t("prefs.choiceNotApplied")}</p>}
    </Card>
  );
}
