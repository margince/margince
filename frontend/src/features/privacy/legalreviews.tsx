// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reviewer's side of a legal review. The queue puts the largest deal
// value first, then the nearest close date. One review shows its pack and the
// form that answers it. Both need the legal_review grant.

import { useState } from "react";
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
import { formatDateTime, formatMoneyOrAbsent } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { calendarDayLabel, LegalReviewPack } from "./legalreviewpack";
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
} from "./legalreviews.queries";
import { LegalReviewStatusBadge } from "./legalreviewstatus";

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
  const zone = viewerZone();
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
  const me = useMe();
  const canDecide = useCan("legal_review", "update");
  const decide = useDecideLegalReview();
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
  const missingComments =
    outcome === "changes_needed" && comments.trim() === "";
  return (
    <Panel title={t("legalReview.decideTitle")}>
      <PanelBody>
        {summary.note && (
          <PanelIntro>
            {t("legalReview.noteShown", { note: summary.note })}
          </PanelIntro>
        )}
        {own && <p className="t-body">{t("legalReview.ownRequest")}</p>}
        <ChoiceList<LegalReviewOutcome>
          legend={t("legalReview.outcome")}
          value={outcome}
          disabled={own || !canDecide}
          onChange={setOutcome}
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
        <Field
          label={t("legalReview.commentsLabel")}
          hint={t("legalReview.commentsHint")}
        >
          {(control) => (
            <Textarea
              {...control}
              value={comments}
              maxLength={8000}
              disabled={own || !canDecide}
              onChange={(event) => setComments(event.target.value)}
            />
          )}
        </Field>
        <div className="card-actions">
          <Button
            variant="primary"
            disabled={
              own ||
              !canDecide ||
              outcome === "" ||
              missingComments ||
              decide.isPending
            }
            onClick={() => {
              if (outcome !== "") {
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
