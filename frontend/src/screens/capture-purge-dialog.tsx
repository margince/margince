import { useEffect, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { usePurgeExclusion } from "./capture-exclusions.queries";
import { problemMessageOf } from "./common";

// Destroying the mail a rule already matched, and saying what that did.
//
// The information sheet promises a colleague that captured mail can be deleted
// "irrevocably, including attachments and derived data". The cascade behind
// that promise is thorough; what the colleague it was made to got back was
// nothing. This is the receipt — and the half that matters most is what was
// KEPT, because a deletion that correctly leaves a Handelsbrief standing looks,
// from the owner's side, exactly like one that silently failed.

type PurgeOutcome = components["schemas"]["CapturePurgeOutcome"];

export function PurgeDialog({
  ruleId,
  ruleValue,
  onClose,
}: Readonly<{ ruleId: string; ruleValue: string; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const purge = usePurgeExclusion();
  // Held here rather than read off purge.data, which React Query clears the
  // moment the next mutation starts. Read from there, pressing "Delete
  // permanently" dropped the preview the reader had just been shown and
  // re-rendered the intro and the Check-first button underneath the
  // confirmation they were in the middle of — and after an error the preview
  // stayed gone, so the dialog forgot what it had told them.
  const [outcome, setOutcome] = useState<PurgeOutcome | null>(null);
  useEffect(() => {
    if (purge.data) {
      setOutcome(purge.data);
    }
  }, [purge.data]);
  // A preview has been seen when one came back saying so; the button below
  // then asks for the real thing.
  const previewed = outcome?.preview === true;

  // Not ConfirmModal: its pair always offers a confirm, and the receipt after
  // the purge has nothing left to confirm.
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("capturePurge.title", { value: ruleValue })}
      </Heading>
      <div className="form-stack">
        {!outcome && <p>{t("capturePurge.intro")}</p>}
        {outcome && <PurgeReceipt outcome={outcome} />}
        {purge.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("capturePurge.failed")}
          >
            {problemMessageOf(purge.error, t)}
          </Callout>
        )}
      </div>
      <div className="actions">
        <span className="actions-pair">
          <Button type="button" onClick={onClose}>
            {previewed || !outcome
              ? t("create.cancel")
              : t("capturePurge.done")}
          </Button>
          {!outcome && (
            <Button
              type="button"
              variant="primary"
              disabled={purge.isPending}
              onClick={() => purge.mutate({ id: ruleId, preview: true })}
            >
              {t("capturePurge.preview")}
            </Button>
          )}
          {previewed && (
            <Button
              type="button"
              variant="danger"
              disabled={purge.isPending}
              onClick={() => purge.mutate({ id: ruleId, preview: false })}
            >
              {t("capturePurge.confirm")}
            </Button>
          )}
        </span>
      </div>
    </Modal>
  );
}

// What the purge did, or would do. The four counts are disjoint and together
// they are every message the rule matched, so a reader can tell "nothing
// matched" from "everything was somebody else's".
function PurgeReceipt({ outcome }: Readonly<{ outcome: PurgeOutcome }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const kept = outcome.kept;
  // Counts are drawn in the reader's own notation, like every other magnitude
  // on screen: a receipt is read as a number, not as a token.
  const n = (count: number) => formatNumber(count, locale);
  return (
    <div className="form-stack">
      <p>
        {outcome.preview
          ? plural("capturePurge.wouldDestroy", outcome.destroyed, {
              count: n(outcome.destroyed),
            })
          : plural("capturePurge.destroyed", outcome.destroyed, {
              count: n(outcome.destroyed),
            })}
      </p>
      {outcome.released > 0 && (
        <p>
          {plural("capturePurge.released", outcome.released, {
            count: n(outcome.released),
          })}
        </p>
      )}
      {outcome.anonymised > 0 && (
        <p>
          {plural("capturePurge.anonymised", outcome.anonymised, {
            count: n(outcome.anonymised),
          })}
        </p>
      )}
      {/* The exception, made legible. Each reason lifts on a different day, so
          they are separate sentences rather than one skipped count. */}
      {kept.held > 0 && (
        <p>
          {plural("capturePurge.keptHeld", kept.held, { count: n(kept.held) })}
        </p>
      )}
      {kept.under_statute > 0 && (
        <p>
          {plural("capturePurge.keptStatute", kept.under_statute, {
            count: n(kept.under_statute),
          })}
          {/* The period is a second sentence rather than a clause, because the
              year-end anchor changes it: "six years" and "six years after the
              end of the calendar year it arrived in" are different promises,
              and only the second is true of a Handelsbrief. Omitted entirely
              when the packs declare a period that is not whole years, since
              there is no number this copy could say truthfully. */}
          {kept.statutory_years !== undefined && kept.statutory_years > 0 && (
            <>
              {" "}
              {plural(
                kept.statutory_from_year_end
                  ? "capturePurge.keptForFromYearEnd"
                  : "capturePurge.keptFor",
                kept.statutory_years,
                { years: n(kept.statutory_years) },
              )}
            </>
          )}
        </p>
      )}
      {kept.under_request > 0 && (
        <p>
          {plural("capturePurge.keptRequest", kept.under_request, {
            count: n(kept.under_request),
          })}
        </p>
      )}
      {/* Its own sentence and never folded into the statutory one. The shield
          is the same act; the basis is not, and a reader told "commercial
          correspondence" has been given a reason nobody established. */}
      {kept.under_undetermined_floor > 0 && (
        <p>
          {plural(
            "capturePurge.keptUndetermined",
            kept.under_undetermined_floor,
            { count: n(kept.under_undetermined_floor) },
          )}
        </p>
      )}
    </div>
  );
}
