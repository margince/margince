// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { entityTimelineKeys } from "./activitykeys";
import { proposalEmailBody } from "./booking-proposal-message";
import { throwProblem } from "./common";
import { ComposeModal } from "./compose";

type Proposal = components["schemas"]["MeetingProposal"];
type Contact = components["schemas"]["Contact360"]["contact"];

/** The reader's own open proposals to one contact: the times still unanswered. */
export function useMeetingProposals(contactId: string | undefined) {
  return useQuery({
    queryKey: ["meeting-proposals", contactId],
    enabled: Boolean(contactId),
    queryFn: async () => {
      const { data, error } = await api.GET("/scheduling/proposals", {
        params: { query: { contact_id: contactId ?? "" } },
      });
      if (error) throwProblem(error);
      return data.data;
    },
  });
}

/**
 * Invitations the contact has not answered yet, between what is booked and
 * what was held. Hidden when there are none, and until the list has arrived:
 * an empty "waiting" section says nothing a reader can act on, and one that
 * shows a heading while loading only to take it away again moves the page.
 */
export function WaitingSection({
  contact,
  proposals,
}: Readonly<{
  contact: Contact;
  proposals: ReturnType<typeof useMeetingProposals>;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [withdrawing, setWithdrawing] = useState<Proposal | null>(null);
  const [resending, setResending] = useState<Proposal | null>(null);
  const withdraw = useMutation({
    mutationFn: async ({
      proposalId,
    }: Readonly<{ proposalId: string; contactId: string }>) => {
      const { error } = await api.DELETE("/activities/{id}", {
        params: { path: { id: proposalId } },
      });
      if (error) throwProblem(error);
    },
    onSuccess: async (_done, { contactId }) => {
      setWithdrawing(null);
      await Promise.all(
        [
          ["meeting-proposals", contactId],
          ...entityTimelineKeys("contact", contactId),
        ].map((queryKey) => client.invalidateQueries({ queryKey })),
      );
    },
  });
  const rows = proposals.data ?? [];
  const state = proposals.isError ? "failed" : "ready";
  if (state === "ready" && rows.length === 0) return null;
  const firstName = contact.first_name?.trim();
  const title = firstName
    ? t("contact.meetings.waitingOn", { name: firstName })
    : t("contact.meetings.waitingOnReply");
  return (
    <section>
      <Heading size="large" className="t-h3">
        {title}
      </Heading>
      <SurfaceState
        loadingLabel={title}
        state={state}
        emptyLabel={title}
        detail={{ onRetry: () => void proposals.refetch() }}
      >
        {rows.map((proposal) => (
          <ProposalRow
            key={proposal.id}
            proposal={proposal}
            onResend={() => setResending(proposal)}
            onWithdraw={() => {
              withdraw.reset();
              setWithdrawing(proposal);
            }}
          />
        ))}
      </SurfaceState>
      <ConfirmModal
        open={withdrawing !== null}
        onClose={() => setWithdrawing(null)}
        title={t("contact.meetings.withdrawTitle")}
        confirmLabel={t("contact.meetings.withdrawConfirm")}
        confirmVariant="danger"
        pending={withdraw.isPending}
        onConfirm={() => {
          if (withdrawing)
            withdraw.mutate({
              proposalId: withdrawing.id,
              contactId: contact.id,
            });
        }}
      >
        <p>{t("contact.meetings.withdrawBody")}</p>
        <ErrorLine error={withdraw.error} />
      </ConfirmModal>
      {resending && (
        <ResendComposer
          contact={contact}
          proposal={resending}
          onClose={() => setResending(null)}
        />
      )}
    </section>
  );
}

// Resending writes the email the proposal was first sent with, from the one
// writer both surfaces share. The list carries no description, so that
// paragraph is left out rather than guessed at.
function ResendComposer({
  contact,
  proposal,
  onClose,
}: Readonly<{ contact: Contact; proposal: Proposal; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const body = proposalEmailBody(
    t,
    { description: "", options: proposal.options, url: proposal.url },
    locale,
    viewerZone(),
  );
  return (
    <ComposeModal
      key={proposal.id}
      entityType="contact"
      entityId={contact.id}
      contactId={contact.id}
      recordAddress={contact.primary_email ?? undefined}
      initialMessage={{ subject: proposal.subject, body }}
      open
      onClose={onClose}
    />
  );
}

function ProposalRow({
  proposal,
  onResend,
  onWithdraw,
}: Readonly<{
  proposal: Proposal;
  onResend: () => void;
  onWithdraw: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const copy = useClipboardCopy(proposal.url, {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  const offered = proposal.options.length;
  const title =
    offered > 0
      ? plural("contact.meetings.proposed", offered, {
          count: formatNumber(offered, locale),
          subject: proposal.subject,
        })
      : t("contact.meetings.personalLink", { subject: proposal.subject });
  const times = proposal.options
    .map(
      (slot) =>
        `${formatDayMonth(slot.start, locale, zone)} ${formatTimeOfDay(slot.start, locale, zone)}`,
    )
    .join(" · ");
  return (
    <article className="pe-waiting">
      <p className="t-body pe-meeting-title">{title}</p>
      {times && <p className="t-num pe-meeting-meta">{times}</p>}
      <p className="t-caption pe-meeting-meta">
        {t("contact.meetings.sentExpires", {
          sent: formatDayMonth(proposal.created_at, locale, zone),
          expires: formatDayMonth(proposal.expires_at, locale, zone),
        })}
      </p>
      <div className="pe-meeting-actions">
        <Button onClick={copy.copy}>{copy.label}</Button>
        <Button onClick={onResend}>{t("contact.meetings.resend")}</Button>
        <Button onClick={onWithdraw}>{t("contact.meetings.withdraw")}</Button>
      </div>
      {copy.notice && (
        <>
          {copy.notice}
          <p className="pe-waiting-url">{proposal.url}</p>
        </>
      )}
    </article>
  );
}
