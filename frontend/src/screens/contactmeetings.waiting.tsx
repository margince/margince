// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDayMonth, formatNumber } from "../format/format";
import { formatTimeRange } from "../format/meetingtime";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { entityTimelineKeys } from "./activitykeys";
import { proposalEmailBody } from "./booking-proposal-message";
import { unwrap } from "./common";
import { ComposeModal } from "./compose";

type Proposal = components["schemas"]["MeetingProposal"];
type Contact = components["schemas"]["Contact360"]["contact"];

/**
 * Which link the tab's one clipboard holds. The tab draws a copy control per
 * link, and only the one copied last may read Copied.
 */
export type CopiedLink = Readonly<{
  url: string | null;
  onCopied: (url: string) => void;
}>;

function meetingProposalsKey(contactId: string | undefined) {
  return ["meeting-proposals", contactId];
}

/**
 * Reads the contact's Meetings tab again after a write that changed what it
 * lists: a meeting booked, an invitation sent or one withdrawn. The tab reads
 * the contact's timeline and the open proposals, so both are stale.
 */
export async function refreshContactMeetings(
  client: QueryClient,
  contactId: string,
): Promise<void> {
  await Promise.all(
    [
      meetingProposalsKey(contactId),
      ...entityTimelineKeys("contact", contactId),
    ].map((queryKey) => client.invalidateQueries({ queryKey })),
  );
}

/** The reader's own open proposals to one contact: the times still unanswered. */
export function useMeetingProposals(contactId: string | undefined) {
  return useQuery({
    queryKey: meetingProposalsKey(contactId),
    enabled: Boolean(contactId),
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/scheduling/proposals", {
          params: { query: { contact_id: contactId ?? "" } },
        }),
      );
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
  copied,
  afterWithdraw,
}: Readonly<{
  contact: Contact;
  proposals: ReturnType<typeof useMeetingProposals>;
  copied: CopiedLink;
  // Where focus lands once a withdrawal removes the card that opened the
  // dialog; the last one takes this whole section with it.
  afterWithdraw: () => HTMLElement | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const withdrawn = useRef(false);
  const [withdrawing, setWithdrawing] = useState<Proposal | null>(null);
  const [resending, setResending] = useState<Proposal | null>(null);
  const withdraw = useMutation({
    mutationFn: async ({
      proposalId,
    }: Readonly<{ proposalId: string; contactId: string }>) => {
      unwrap(
        await api.DELETE("/activities/{id}", {
          params: { path: { id: proposalId } },
        }),
      );
    },
    onSuccess: async (_done, { contactId }) => {
      withdrawn.current = true;
      setWithdrawing(null);
      await refreshContactMeetings(client, contactId);
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
    <Panel
      title={title}
      titleAction={
        state === "ready" && <Badge>{formatNumber(rows.length, locale)}</Badge>
      }
    >
      {state === "failed" ? (
        <PanelBody>
          <SurfaceState
            loadingLabel={title}
            state={state}
            emptyLabel={title}
            detail={{ onRetry: () => void proposals.refetch() }}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      ) : (
        rows.map((proposal) => (
          <ProposalRow
            key={proposal.id}
            proposal={proposal}
            copied={copied}
            onResend={() => setResending(proposal)}
            onWithdraw={() => {
              withdrawn.current = false;
              withdraw.reset();
              setWithdrawing(proposal);
            }}
          />
        ))
      )}
      <ConfirmModal
        open={withdrawing !== null}
        onClose={() => setWithdrawing(null)}
        returnFocusTo={() => (withdrawn.current ? afterWithdraw() : null)}
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
    </Panel>
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
  copied,
  onResend,
  onWithdraw,
}: Readonly<{
  proposal: Proposal;
  copied: CopiedLink;
  onResend: () => void;
  onWithdraw: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const copy = useClipboardCopy(
    proposal.url,
    {
      copy: t("scheduling.copyLink"),
      copied: t("scheduling.copied"),
      remedy: t("scheduling.copyFallback"),
    },
    () => copied.onCopied(proposal.url),
  );
  return (
    <PanelRow>
      <article className="pe-proposal">
        <div className="pe-meeting-body">
          <div className="pe-meeting-headline">
            <span className="pe-meeting-title">{proposal.subject}</span>
            {proposal.options.length === 0 && (
              <Badge>{t("contact.meetings.personalLink")}</Badge>
            )}
          </div>
          {proposal.options.length > 0 && (
            <ul
              className="pe-proposal-times"
              aria-label={t("contact.meetings.offeredTimes")}
            >
              {proposal.options.map((slot) => (
                <li key={slot.start}>
                  <Badge>
                    {formatDayMonth(slot.start, locale, zone)} ·{" "}
                    {formatTimeRange(slot.start, slot.end, locale, zone)}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
          <p className="t-caption pe-meeting-meta">
            {t("contact.meetings.sentExpires", {
              sent: formatDayMonth(proposal.created_at, locale, zone),
              expires: formatDayMonth(proposal.expires_at, locale, zone),
            })}
          </p>
        </div>
        <div className="pe-meeting-verbs">
          <Button onClick={copy.copy}>
            {t(
              copied.url === proposal.url
                ? "scheduling.copied"
                : "scheduling.copyLink",
            )}
          </Button>
          <Button onClick={onResend}>{t("contact.meetings.resend")}</Button>
          {/* Withdrawing is the rare verb, and the one that cannot be undone:
              folded away from the two a reader reaches for. */}
          <OverflowMenu
            label={t("contact.meetings.moreFor", { subject: proposal.subject })}
          >
            <Button variant="ghost" onClick={onWithdraw}>
              {t("contact.meetings.withdraw")}
            </Button>
          </OverflowMenu>
        </div>
        {copy.notice && (
          <div className="pe-copy-notice pe-proposal-notice">
            {copy.notice}
            <p className="pe-copy-url">{proposal.url}</p>
          </div>
        )}
      </article>
    </PanelRow>
  );
}
