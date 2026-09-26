import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ErrorLine } from "../design-system/errorline";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useBookingIntent } from "./booking-common";
import { throwProblem } from "./common";
import { ComposeModal } from "./compose";

type Proposal = components["schemas"]["MeetingProposalRequest"];
export function BookingProposal({
  request,
  zone,
  personalOnly,
}: Readonly<{ request: Proposal; zone: string; personalOnly: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const intent = useBookingIntent();
  const [review, setReview] = useState(false);
  const create = useMutation({
    mutationFn: async (body: Proposal) => {
      const { data, error } = await api.POST("/scheduling/proposals", {
        params: { header: { "Idempotency-Key": intent(body) } },
        body,
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const copy = useClipboardCopy(create.data?.url ?? "", {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  const offered = create.variables ?? request;
  const changed =
    create.data && JSON.stringify(offered) !== JSON.stringify(request);
  const options = offered.options
    .map((slot) => formatDateTime(slot.start, locale, zone))
    .join("\n");
  const message = `${t("scheduling.proposalGreeting")}\n\n${offered.description}\n\n${options}\n\n${t("scheduling.proposalChoose")}\n${create.data?.url ?? ""}\n\n${t("scheduling.proposalReply")}`;
  return (
    <div className="book-form">
      <p className="t-caption">{t("scheduling.proposalHelp")}</p>
      {!create.data ? (
        <Button
          variant="primary"
          disabled={
            !request.contact_id ||
            !request.attendee_email ||
            !request.subject ||
            (!personalOnly && request.options.length < 2) ||
            create.isPending
          }
          onClick={() => create.mutate(request)}
        >
          {t(
            personalOnly
              ? "scheduling.createPersonalLink"
              : "scheduling.reviewProposal",
          )}
        </Button>
      ) : (
        <>
          {changed && (
            <div className="book-form">
              <p>{t("scheduling.proposalChanged")}</p>
              <Button onClick={() => create.reset()}>
                {t("scheduling.proposalUpdate")}
              </Button>
            </div>
          )}
          <a href={create.data.url}>{t("scheduling.personalLink")}</a>
          <p className="t-caption">
            {t("scheduling.expires", {
              date: formatDateTime(create.data.expires_at, locale, zone),
            })}
          </p>
          <div className="book-actions">
            <Button onClick={copy.copy}>{copy.label}</Button>
            <Button variant="primary" onClick={() => setReview(true)}>
              {t("scheduling.reviewEmail")}
            </Button>
          </div>
          {copy.notice}
          {review && (
            <ComposeModal
              key={create.data.id}
              entityType="contact"
              entityId={offered.contact_id}
              contactId={offered.contact_id}
              recordAddress={offered.attendee_email}
              initialMessage={{ subject: offered.subject, body: message }}
              open
              onClose={() => setReview(false)}
            />
          )}
        </>
      )}
      <ErrorLine error={create.error} />
    </div>
  );
}
