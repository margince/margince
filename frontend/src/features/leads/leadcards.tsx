import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
  OverviewText,
} from "../../design-system/overviewcard";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { emailSummaryText } from "../../format/emailtext";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Lead } from "./leads";
import { type PromotionRecord, promotionSummary } from "./leads.promotion";
import { usePromotePreview } from "./leads.qualify";
import {
  evidenceText,
  LeadManualSignals,
  useLeadSignalCatalog,
  useManualSignals,
} from "./leadsignals";

type Activity = components["schemas"]["Activity"];

// The oldest message from the lead, once the page holds its whole history.
// On a cut page the oldest row in hand is not the first they sent.
export function firstMessage(timeline: RecordTimeline): Activity | undefined {
  if (timeline.hasNextPage) {
    return undefined;
  }
  return timeline.activities
    .filter(
      (row) =>
        row.direction === "inbound" &&
        row.content_state !== "withheld" &&
        Boolean(row.body?.trim() || row.subject?.trim()),
    )
    .sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at))[0];
}

/**
 * What they asked for: the lead's first message in their own words. A lead
 * carries no typed enquiry, so a lead whose first message is not on record
 * has no card rather than an invented quote.
 */
export function LeadEnquiryCard({
  thread,
  onOpenEmail,
}: Readonly<{
  thread: RecordTimeline;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const message = firstMessage(thread);
  if (!message) {
    return null;
  }
  // Their words go in quotes; a subject line standing in for them does not.
  const body = message.body?.trim();
  const words = body
    ? t("lead.card.askedQuote", { text: emailSummaryText(body) })
    : (message.subject ?? "");
  return (
    <OverviewCard
      title={t("lead.card.asked")}
      action={
        message.kind === "email" ? (
          <Button variant="link" onClick={() => onOpenEmail(message.id)}>
            {t("lead.card.askedOpen")}
          </Button>
        ) : undefined
      }
    >
      <OverviewText>
        <span className="lead-enquiry">{words}</span>
      </OverviewText>
    </OverviewCard>
  );
}

/**
 * If they are qualified: what the server's promotion preview says happens to
 * the contact, and that a deal can be opened in the same step. The preview
 * names no deal, so the card names none either. Once the lead is qualified the
 * same card says "Qualified as" and links the contact it became.
 */
export function LeadQualifyPreviewCard({
  lead,
  promotion,
}: Readonly<{ lead: Lead; promotion: PromotionRecord }>) {
  const t = useT();
  const open =
    !lead.archived_at &&
    !lead.merged_into_id &&
    lead.status !== "promoted" &&
    lead.status !== "disqualified";
  const preview = usePromotePreview(lead.id, open);
  if (lead.promoted_contact_id && !lead.merged_into_id) {
    return (
      <OverviewCard title={t("lead.card.qualifiedAs")}>
        <OverviewList>
          <OverviewRow
            title={t("lead.card.qualifiedContact")}
            sub={promotionSummary(promotion, t)}
            right={<EntityRef kind="contact" id={lead.promoted_contact_id} />}
          />
        </OverviewList>
      </OverviewCard>
    );
  }
  if (!open) {
    return null;
  }
  return (
    <OverviewCard title={t("lead.card.qualified")}>
      <OverviewList>
        <OverviewRow
          title={t("lead.card.qualifiedContact")}
          sub={<PreviewContact preview={preview} />}
        />
        <OverviewRow
          title={t("lead.card.qualifiedDeal")}
          sub={t("lead.card.qualifiedDealOptional")}
        />
      </OverviewList>
    </OverviewCard>
  );
}

function PreviewContact({
  preview,
}: Readonly<{ preview: ReturnType<typeof usePromotePreview> }>) {
  const t = useT();
  if (preview.isPending) {
    return t("lead.previewPending");
  }
  if (preview.isError || !preview.data) {
    return t("lead.card.qualifiedUnknown");
  }
  if (preview.data.outcome === "create") {
    return t("lead.card.qualifiedNewContact");
  }
  // An absent contact on a merge is one outside the reader's row scope, not
  // the absence of a match.
  if (!preview.data.contact) {
    return t("lead.previewMergeWithheld");
  }
  return (
    <>
      {t("lead.card.qualifiedMergesInto")}{" "}
      <EntityRef kind="contact" id={preview.data.contact.id} />
    </>
  );
}

type ManualSignal = components["schemas"]["LeadManualSignal"];

// Who entered a signal. `set_by` is the user who typed it in, never the
// source they heard it from, so the row says "Set by", not "Told by".
function SetBy({ userId }: Readonly<{ userId: string }>) {
  const t = useT();
  return (
    <>
      {t("lead.signal.setBy")} <EntityRef kind="user" id={userId} asText />
    </>
  );
}

function KnowRow({
  factor,
  live,
}: Readonly<{ factor: string; live?: ManualSignal }>) {
  const t = useT();
  const label = (key: string) => t(`lead.signal.${key}` as MessageKey);
  const band = live ? label(`${factor}.${live.band}`) : undefined;
  return (
    <OverviewRow
      title={label(factor)}
      sub={
        live
          ? [band, evidenceText(live.reason, t)].filter(Boolean).join(" · ")
          : t("lead.signal.notAsked")
      }
      right={live ? <SetBy userId={live.set_by} /> : undefined}
    />
  );
}

/**
 * What you know: one row per factor a rep may add to the score, with the
 * band on record or that nobody has entered one. The form that adds them
 * opens from the card's link.
 */
export function LeadKnowCard({
  id,
  readOnlyReason,
  reasonId,
}: Readonly<{ id: string; readOnlyReason?: string; reasonId?: string }>) {
  const t = useT();
  const titleId = useId();
  const [adding, setAdding] = useState(false);
  const signals = useManualSignals(id);
  const catalog = useLeadSignalCatalog();
  const factors = catalog.data ?? [];
  return (
    <OverviewCard
      title={t("lead.card.know")}
      action={
        <Button
          variant="link"
          reasonId={reasonId}
          onClick={() => setAdding(true)}
        >
          {t("lead.card.knowAdd")}
        </Button>
      }
    >
      {(signals.isPending || catalog.isPending) && (
        <OverviewText>{t("lead.scoreLoading")}</OverviewText>
      )}
      {(signals.isError || catalog.isError) && (
        <ErrorLine error={signals.error ?? catalog.error} />
      )}
      {signals.isSuccess && catalog.isSuccess && (
        <OverviewList>
          {factors.map(({ factor }) => (
            <KnowRow
              key={factor}
              factor={factor}
              live={signals.data.find(
                (entry) => entry.factor === factor && !entry.superseded_at,
              )}
            />
          ))}
        </OverviewList>
      )}
      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        labelledBy={titleId}
        intent="form"
      >
        <Heading size="large" className="t-h3 modal-title" id={titleId}>
          {t("lead.card.know")}
        </Heading>
        {/* Keyed by lead: a half-typed answer for one lead must not be
            submitted against the next one the reader navigates to. */}
        <LeadManualSignals
          key={id}
          id={id}
          readOnlyReason={readOnlyReason}
          onSaved={() => setAdding(false)}
        />
      </Modal>
    </OverviewCard>
  );
}
