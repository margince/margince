// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { Button, EmptyState, Modal } from "../../design-system/atoms";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  captureReviewLink,
  captureReviewTarget,
} from "../../lib/capturereview";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { useCaptureHealth } from "./capturehealth";

type Review = "senders" | "held";
type Owner = components["schemas"]["CaptureOwnerReview"];

export function MailReviewRequests({
  review,
  primary = false,
}: Readonly<{ review: Review; primary?: boolean }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const triggerId = useId();
  return (
    <>
      <Button
        id={triggerId}
        variant={primary ? "primary" : undefined}
        onClick={() => setOpen(true)}
      >
        {t(primary ? "health.code.capture_owner_review" : "health.askOwner")}
      </Button>
      {open && (
        <Modal
          open
          intent="drawer"
          labelledBy={titleId}
          returnFocusTo={() => document.getElementById(triggerId)}
          onClose={() => setOpen(false)}
        >
          <DrawerHead>
            <Heading id={titleId} size="medium" as="h2" className="modal-title">
              {t("health.mailReviewOwners")}
            </Heading>
          </DrawerHead>
          <DrawerBody>
            <ReviewOwners review={review} />
          </DrawerBody>
        </Modal>
      )}
    </>
  );
}

function openReview(review: Review) {
  const target = captureReviewTarget(review);
  navigate(target.route, target.params);
}

function ReviewOwners({ review }: Readonly<{ review: Review }>) {
  const t = useT();
  const { canSee, query } = useCaptureHealth();
  const me = useMe();
  const fallback = (
    <div className="form-stack">
      <p>{t("health.ownerReviewUnavailable")}</p>
      <Button variant="primary" onClick={() => openReview(review)}>
        {t("health.openMyReviews")}
      </Button>
      <ReviewRequestCopy review={review} name={t("health.ownerNameUnknown")} />
    </div>
  );
  if (!canSee) return fallback;
  return (
    <div className="form-stack">
      <p>{t("health.reviewOwnersIntro")}</p>
      <QueryGate query={query} pendingLabel={t("health.mailReviewOwners")}>
        {(report) => {
          if (report.owner_reviews === undefined) return fallback;
          const owners = report.owner_reviews.filter((owner) =>
            review === "senders"
              ? owner.sender_decisions > 0
              : owner.thread_decisions > 0,
          );
          return owners.length === 0 ? (
            <EmptyState>{t("health.ownerReviewEmpty")}</EmptyState>
          ) : (
            <SettingList>
              {owners.map((owner) => (
                <ReviewOwner
                  key={owner.user_id}
                  owner={owner}
                  review={review}
                  mine={owner.user_id === me.data?.user.id}
                />
              ))}
            </SettingList>
          );
        }}
      </QueryGate>
    </div>
  );
}

function ReviewOwner({
  owner,
  review,
  mine,
}: Readonly<{ owner: Owner; review: Review; mine: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const count =
    review === "senders" ? owner.sender_decisions : owner.thread_decisions;
  const name = owner.display_name ?? t("health.ownerNameUnknown");
  return (
    <SettingRow
      label={name}
      description={plural(
        review === "senders"
          ? "health.senderReviewCount"
          : "health.threadReviewCount",
        count,
        { count: formatNumber(count, locale) },
      )}
      control={
        mine ? (
          <Button variant="primary" onClick={() => openReview(review)}>
            {t("health.openMyReviews")}
          </Button>
        ) : (
          <ReviewRequestCopy review={review} name={name} />
        )
      }
    />
  );
}

function ReviewRequestCopy({
  review,
  name,
}: Readonly<{ review: Review; name: string }>) {
  const t = useT();
  const text = t(
    review === "senders"
      ? "health.reviewSenderRequest"
      : "health.reviewThreadRequest",
    {
      name,
      url: captureReviewLink(review, window.location.href),
    },
  );
  const copy = useClipboardCopy(text, {
    copy: t("health.copyReviewRequest"),
    copied: t("health.copied"),
    remedy: t("health.copyRequestRemedy"),
  });
  return (
    <div className="form-stack">
      <Button
        onClick={copy.copy}
        aria-label={t("health.copyReviewRequestFor", { name })}
      >
        {copy.label}
      </Button>
      {copy.notice}
      {copy.notice && (
        <CopyableText text={text} label={t("health.copyReviewRequest")} />
      )}
    </div>
  );
}
