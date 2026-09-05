import type { components } from "../api/schema";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";

// When the pass a person is waiting on will run.
//
// A screen that says "waiting" and nothing else leaves two readings, broken and
// slow, and both are wrong: the pipeline is working exactly as declared, on a
// clock the screen never mentioned. The wait is longest exactly when trust is
// lowest — the first minutes after somebody connects their own mailbox — and an
// unexplained hour there reads as an integration that does not work.
//
// Two passes use this, an hour apart in cadence, and that is the second half of
// the same complaint: "waiting on a verdict" named neither which verdict nor
// which clock. The subject is a parameter for that reason, not for reuse's own
// sake.

type Clock = components["schemas"]["CaptureVerdictClock"];

// SUBJECT is the plural noun the sentence is about. Both catalogs' sentences
// take it in the same position and neither language inflects the verb for it,
// which is what makes one sentence safe for both.
type Subject = "senders" | "threads";

export function VerdictPassNote({
  clock,
  subject,
}: Readonly<{ clock: Clock | undefined; subject: Subject }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (!clock || clock.every_seconds <= 0) {
    // No clock runs this pass, or this deployment cannot say. Silence is the
    // honest answer: a cadence nobody keeps is worse than no sentence at all.
    return null;
  }
  const minutes = Math.round(clock.every_seconds / 60);
  return (
    <p className="t-sub" data-testid={`verdict-pass-${subject}`}>
      {plural("verdictPass.every", minutes, {
        subject: t(`verdictPass.subject.${subject}`),
        minutes: formatNumber(minutes, locale),
      })}{" "}
      {passState(clock, t, locale)}
    </p>
  );
}

// Three states, and only one of them is a time. A pass in flight says so; a pass
// that is DUE and waiting for a worker says that, because its moment has already
// passed and printing it would name a time that has been and gone — and it is
// the state that tells a slow installation from a stopped one, which is what
// somebody watching an unmoving counter is really asking.
function passState(
  clock: Clock,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
): string {
  if (clock.running) {
    return t("verdictPass.running");
  }
  if (clock.queued) {
    return t("verdictPass.queued");
  }
  if (!clock.next_pass_at) {
    return "";
  }
  return t("verdictPass.next", {
    when: formatDateTime(clock.next_pass_at, locale, viewerZone()),
  });
}
