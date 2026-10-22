// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A deal's legal reviews: the request, what legal answered, and the verb to
// ask again after "changes needed". The pack itself is the reviewer's to
// open; this tab shows the deal's side of the exchange.

import { useState } from "react";
import type { components } from "../../../api/schema";
import { LegalReviewStatusBadge } from "../../../components/legalreviewstatus";
import {
  Button,
  EmptyState,
  Field,
  Textarea,
} from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { DataTable } from "../../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../../design-system/panel";
import { QueryGate } from "../../../design-system/querystates";
import { formatDateTime } from "../../../format/format";
import { viewerZone } from "../../../format/timezone";
import { useLocale, useT } from "../../../i18n";
import { useCan, useCanWriteRecord } from "../../../lib/capability";
import {
  LEGAL_REVIEWS_SCREEN,
  type LegalReviewSummary,
  useDealLegalReviews,
  useLegalReviewAvailable,
  useRequestLegalReview,
  useWithdrawLegalReview,
} from "../../../lib/legalreviews.queries";
import { problemMessageOf } from "../../../lib/problem";
import { navigate } from "../../../lib/router";
import "./deallegaltab.css";

type Deal = components["schemas"]["Deal"];

/**
 * The verb that asks legal, on the deal header and in the tab. Draws nothing
 * while the feature is off or the reader may not change the deal, so it never
 * offers a request the server refuses.
 */
export function RequestLegalReviewAction({
  deal,
  previousReviewId,
}: Readonly<{ deal: Deal; previousReviewId?: string }>) {
  const t = useT();
  const available = useLegalReviewAvailable();
  const canWrite = useCanWriteRecord("deal", deal);
  const [open, setOpen] = useState(false);
  if (!available || !canWrite || deal.archived_at) {
    return null;
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {previousReviewId
          ? t("legalReview.requestAgain")
          : t("legalReview.request")}
      </Button>
      {open && (
        <RequestDialog
          dealId={deal.id}
          previousReviewId={previousReviewId}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function RequestDialog({
  dealId,
  previousReviewId,
  onClose,
}: Readonly<{
  dealId: string;
  previousReviewId?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const request = useRequestLegalReview();
  const [note, setNote] = useState("");
  return (
    <ConfirmModal
      open
      onClose={onClose}
      intent="form"
      title={t("legalReview.request")}
      confirmLabel={t("legalReview.requestConfirm")}
      pending={request.isPending}
      error={request.error ? problemMessageOf(request.error, t) : null}
      onConfirm={() => {
        request
          .mutateAsync({ dealId, note, previousReviewId })
          .then(onClose)
          // The refusal is drawn from the mutation's own error state above.
          .catch(() => undefined);
      }}
    >
      <p className="t-body">{t("legalReview.packIntro")}</p>
      <Field
        label={t("legalReview.noteLabel")}
        hint={t("legalReview.noteHint")}
      >
        {(control) => (
          <Textarea
            {...control}
            value={note}
            maxLength={4000}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

/** The deal's legal review tab: the request, then every answer legal gave. */
export function DealLegalTab({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const available = useLegalReviewAvailable();
  const reviews = useDealLegalReviews(deal.id, available === true);
  const newest = reviews.data?.[0];
  return (
    <>
      <Panel title={t("legalReview.tab")}>
        <PanelBody>
          <PanelIntro>{t("legalReview.packIntro")}</PanelIntro>
          {reviews.isSuccess && newest?.status !== "requested" && (
            <div className="card-actions">
              <RequestLegalReviewAction
                deal={deal}
                previousReviewId={
                  newest?.status === "changes_needed" ? newest.id : undefined
                }
              />
            </div>
          )}
        </PanelBody>
      </Panel>
      <Panel title={t("legalReview.reviews")}>
        <QueryGate query={reviews} pendingLabel={t("legalReview.reviews")}>
          {(list) =>
            list.length === 0 ? (
              <PanelBody>
                <EmptyState>{t("legalReview.empty")}</EmptyState>
              </PanelBody>
            ) : (
              <PanelBody>
                <ReviewTable deal={deal} reviews={list} />
              </PanelBody>
            )
          }
        </QueryGate>
      </Panel>
    </>
  );
}

function ReviewTable({
  deal,
  reviews,
}: Readonly<{ deal: Deal; reviews: readonly LegalReviewSummary[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const someone = t("legalReview.someoneShort");
  return (
    <DataTable
      label={t("legalReview.reviews")}
      fold
      rows={[...reviews]}
      rowKey={(review) => review.id}
      columns={[
        {
          key: "status",
          header: t("legalReview.col.status"),
          fold: "title",
          render: (review) => <LegalReviewStatusBadge status={review.status} />,
        },
        {
          key: "requested",
          header: t("legalReview.col.requestedBy"),
          render: (review) => review.requested_by_name ?? someone,
        },
        {
          key: "when",
          header: t("legalReview.col.when"),
          render: (review) => formatDateTime(review.created_at, locale, zone),
        },
        {
          key: "decided",
          header: t("legalReview.col.decidedBy"),
          render: (review) =>
            review.decided_at ? (review.decided_by_name ?? someone) : null,
        },
        {
          key: "comments",
          header: t("legalReview.col.comments"),
          grow: true,
          render: (review) => <ReviewWords review={review} />,
        },
        {
          key: "actions",
          header: t("table.actions"),
          headerHidden: true,
          fold: "end",
          render: (review) => <ReviewVerbs deal={deal} review={review} />,
        },
      ]}
    />
  );
}

// What legal answered, and under it what the requester asked them to look at.
function ReviewWords({ review }: Readonly<{ review: LegalReviewSummary }>) {
  const t = useT();
  if (!review.comments && !review.note) {
    return null;
  }
  return (
    <span className="legal-review-words">
      {review.comments && <span>{review.comments}</span>}
      {review.note && (
        <span className="t-caption">
          {t("legalReview.noteShown", { note: review.note })}
        </span>
      )}
    </span>
  );
}

function ReviewVerbs({
  deal,
  review,
}: Readonly<{ deal: Deal; review: LegalReviewSummary }>) {
  const t = useT();
  const reviewer = useCan("legal_review", "read");
  const canWrite = useCanWriteRecord("deal", deal);
  return (
    <span className="cell-actions">
      {reviewer && (
        <Button
          variant="link"
          onClick={() =>
            navigate({ screen: LEGAL_REVIEWS_SCREEN, id: review.id })
          }
        >
          {t("legalReview.openPack")}
        </Button>
      )}
      {review.status === "requested" && canWrite && (
        <WithdrawAction reviewId={review.id} />
      )}
    </span>
  );
}

function WithdrawAction({ reviewId }: Readonly<{ reviewId: string }>) {
  const t = useT();
  const withdraw = useWithdrawLegalReview();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("legalReview.withdraw")}</Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("legalReview.withdrawTitle")}
        confirmLabel={t("legalReview.withdraw")}
        pending={withdraw.isPending}
        error={withdraw.error ? problemMessageOf(withdraw.error, t) : null}
        onConfirm={() => {
          withdraw
            .mutateAsync(reviewId)
            .then(() => setOpen(false))
            .catch(() => undefined);
        }}
      >
        <p className="t-body">{t("legalReview.withdrawText")}</p>
      </ConfirmModal>
    </>
  );
}
