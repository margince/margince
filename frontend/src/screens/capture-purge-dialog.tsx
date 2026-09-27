import { useId } from "react";
import type { components } from "../api/schema";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { usePurgeExclusion } from "./capture-exclusions.queries";
import { problemMessageOf } from "./common";

// Destroying the mail a rule already matched, and saying what that did.
//
// The information sheet promises a colleague that captured mail can be deleted
// "irrevocably, including attachments and derived data". The cascade behind
// that promise is thorough; what the person it was made to got back was
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
  const outcome = purge.data;
  // A preview has been seen when one came back saying so; the button below
  // then asks for the real thing.
  const previewed = outcome?.preview === true;

  return (
    <Modal open onClose={onClose} labelledBy={headingId}>
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
        <div className="form-actions">
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
        </div>
      </div>
    </Modal>
  );
}

// What the purge did, or would do. The four counts are disjoint and together
// they are every message the rule matched, so a reader can tell "nothing
// matched" from "everything was somebody else's".
function PurgeReceipt({ outcome }: Readonly<{ outcome: PurgeOutcome }>) {
  const t = useT();
  const kept = outcome.kept;
  return (
    <div className="form-stack">
      <p>
        {outcome.preview
          ? t("capturePurge.wouldDestroy", { count: String(outcome.destroyed) })
          : t("capturePurge.destroyed", { count: String(outcome.destroyed) })}
      </p>
      {outcome.released > 0 && (
        <p>{t("capturePurge.released", { count: String(outcome.released) })}</p>
      )}
      {outcome.anonymised > 0 && (
        <p>
          {t("capturePurge.anonymised", { count: String(outcome.anonymised) })}
        </p>
      )}
      {/* The exception, made legible. Each reason lifts on a different day, so
          they are separate sentences rather than one skipped count. */}
      {kept.held > 0 && (
        <p>{t("capturePurge.keptHeld", { count: String(kept.held) })}</p>
      )}
      {kept.under_statute > 0 && (
        <p>
          {t("capturePurge.keptStatute", {
            count: String(kept.under_statute),
            period: kept.statutory_period ?? "",
          })}
        </p>
      )}
      {kept.under_request > 0 && (
        <p>
          {t("capturePurge.keptRequest", { count: String(kept.under_request) })}
        </p>
      )}
    </div>
  );
}
