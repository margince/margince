// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reviewer's side of a legal review. The queue puts the largest deal
// value first, then the nearest close date. One review shows its pack and the
// form that answers it. Both need the legal_review grant.

import { useState } from "react";
import { LegalReviewStatusBadge } from "../../components/legalreviewstatus";
import {
  Button,
  EmptyState,
  Field,
  SectionHeader,
  SegmentedControl,
  Textarea,
} from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelRow,
} from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { formatDateTime, formatMoneyOrAbsent } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import {
  LEGAL_REVIEWS_SCREEN,
  type LegalReview,
  type LegalReviewOutcome,
  type LegalReviewQueueItem,
  type QueueStatus,
  useDecideLegalReview,
  useLegalReview,
  useLegalReviewAvailable,
  useLegalReviewQueue,
} from "../../lib/legalreviews.queries";
import { blankRequired } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { useViewerZone } from "../../lib/viewerzone";
import { calendarDayLabel, LegalReviewPack } from "./legalreviewpack";

/** The door from the deals page, for a reader who reviews. */
export function LegalReviewQueueLink() {
  const t = useT();
  const available = useLegalReviewAvailable();
  const reviewer = useCan("legal_review", "read");
  if (!available || !reviewer) {
    return null;
  }
  return (
    <Button onClick={() => navigate({ screen: LEGAL_REVIEWS_SCREEN })}>
      {t("legalReview.queueTitle")}
    </Button>
  );
}

const QUEUE_STATUSES: readonly QueueStatus[] = ["open", "decided"];

/** #/legal-reviews and #/legal-reviews/<id>. */
export function LegalReviewsRoute({ id }: Readonly<{ id?: string }>) {
  return id ? <LegalReviewScreen id={id} /> : <LegalReviewsScreen />;
}

function LegalReviewsScreen() {
  const t = useT();
  const [status, setStatus] = useState<QueueStatus>("open");
  const queue = useLegalReviewQueue(status, true);
  const rows = queue.data?.pages.flatMap((page) => page.data);
  return (
    <div className="wrap">
      <SegmentedControl
        options={QUEUE_STATUSES}
        value={status}
        onChange={setStatus}
        label={t("legalReview.queueTitle")}
        labels={{
          open: t("legalReview.queueOpen"),
          decided: t("legalReview.queueDecided"),
        }}
      />
      <Panel>
        <QueryStates query={queue} pendingLabel={t("legalReview.queueTitle")}>
          {rows && rows.length === 0 && (
            <PanelBody>
              <EmptyState>
                {status === "open"
                  ? t("legalReview.queueEmptyOpen")
                  : t("legalReview.queueEmptyDecided")}
              </EmptyState>
            </PanelBody>
          )}
          {rows?.map((item) => (
            <QueueRow key={item.review.id} item={item} />
          ))}
        </QueryStates>
      </Panel>
      {queue.hasNextPage && (
        <Button
          disabled={queue.isFetchingNextPage}
          onClick={() => void queue.fetchNextPage()}
        >
          {t("list.loadMore")}
        </Button>
      )}
    </div>
  );
}

function QueueRow({ item }: Readonly<{ item: LegalReviewQueueItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const open = () =>
    navigate({ screen: LEGAL_REVIEWS_SCREEN, id: item.review.id });
  return (
    <PanelRow className="meta-row" interactive record onOpen={open}>
      <span className="meta-row-line t-caption">
        <LegalReviewStatusBadge status={item.review.status} />
        {item.company_name && <span>{item.company_name}</span>}
        {item.stage_name && <span>{item.stage_name}</span>}
        <span>
          {formatMoneyOrAbsent(item.value_minor, item.value_currency, locale)}
        </span>
        {item.expected_close_date && (
          <span>
            {t("legalReview.closesOn", {
              date: calendarDayLabel(item.expected_close_date, locale, zone),
            })}
          </span>
        )}
        <span>{formatDateTime(item.review.created_at, locale, zone)}</span>
      </span>
      <span className="meta-row-entry t-body">
        <Button variant="link" onClick={open}>
          {item.deal_name}
        </Button>
      </span>
    </PanelRow>
  );
}

function LegalReviewScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const review = useLegalReview(id);
  return (
    <div className="wrap">
      <Button
        variant="link"
        onClick={() => navigate({ screen: LEGAL_REVIEWS_SCREEN })}
      >
        {t("legalReview.back")}
      </Button>
      <QueryGate query={review} pendingLabel={t("legalReview.packTitle")}>
        {(data) => (
          <div className="record-stack">
            <SectionHeader title={data.packet.deal.name} />
            <DecidePanel review={data} />
            <LegalReviewPack review={data} />
          </div>
        )}
      </QueryGate>
    </div>
  );
}

function DecidePanel({ review }: Readonly<{ review: LegalReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const me = useMe();
  const canDecide = useCan("legal_review", "update");
  const decide = useDecideLegalReview();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [outcome, setOutcome] = useState<LegalReviewOutcome | "">("");
  const [comments, setComments] = useState("");
  const summary = review.review;
  const own = me.data?.user?.id === summary.requested_by;
  if (summary.status !== "requested") {
    return (
      <Panel title={t("legalReview.decideTitle")}>
        <PanelBody>
          <PanelIntro>
            <LegalReviewStatusBadge status={summary.status} />
          </PanelIntro>
          {summary.comments && (
            <p className="t-body">
              {t("legalReview.commentsShown", { comments: summary.comments })}
            </p>
          )}
        </PanelBody>
      </Panel>
    );
  }
  const labels = {
    outcome: t("legalReview.outcome"),
    comments: t("legalReview.commentsLabel"),
  };
  // Changes asked for owe the requester what to change.
  const blank = () =>
    blankRequired(locale, {
      outcome: [labels.outcome, outcome],
      ...(outcome === "changes_needed"
        ? { comments: [labels.comments, comments] as const }
        : {}),
    });
  return (
    <Panel title={t("legalReview.decideTitle")}>
      <PanelBody>
        {summary.note && (
          <PanelIntro>
            {t("legalReview.noteShown", { note: summary.note })}
          </PanelIntro>
        )}
        {own && <p className="t-body">{t("legalReview.ownRequest")}</p>}
        <div className="form-stack" ref={faultsRoot}>
          <ChoiceList<LegalReviewOutcome>
            legend={t("legalReview.outcome")}
            value={outcome}
            disabled={own || !canDecide}
            onChange={(next) => {
              setOutcome(next);
              faults.edited("outcome");
              faults.edited("comments");
            }}
            choices={[
              {
                value: "approved",
                label: t("legalReview.outcomeApproved"),
                description: t("legalReview.outcomeApprovedHint"),
              },
              {
                value: "changes_needed",
                label: t("legalReview.outcomeChanges"),
                description: t("legalReview.outcomeChangesHint"),
              },
            ]}
          />
          <ErrorLine>{faults.of("outcome")}</ErrorLine>
          <Field
            label={labels.comments}
            hint={t("legalReview.commentsHint")}
            required={outcome === "changes_needed"}
            error={faults.of("comments")}
          >
            {(control) => (
              <Textarea
                {...control}
                value={comments}
                maxLength={TEXT_LIMITS.note}
                disabled={own || !canDecide}
                onChange={(event) => {
                  setComments(event.target.value);
                  faults.edited("comments");
                }}
              />
            )}
          </Field>
        </div>
        <div className="card-actions">
          <Button
            variant="primary"
            disabled={own || !canDecide || decide.isPending}
            onClick={() => {
              if (!faults.show(blank()) && outcome !== "") {
                decide.mutate({ id: summary.id, outcome, comments });
              }
            }}
          >
            {t("legalReview.decide")}
          </Button>
        </div>
        <ErrorLine error={decide.error} />
      </PanelBody>
    </Panel>
  );
}
