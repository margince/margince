import { useMutation, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import {
  Button,
  Disclosure,
  Field,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ErrorLine } from "../design-system/errorline";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordPicker } from "../design-system/recordpicker";
import { Switch } from "../design-system/switch";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useBookingIntent } from "./booking-common";
import type { BookingMode, BookingSlot } from "./booking-picker";
import { proposalEmailBody } from "./booking-proposal-message";
import { throwProblem } from "./common";
import { ComposeModal } from "./compose";
import { refreshContactMeetings } from "./contactmeetings.waiting";

type Invitation = components["schemas"]["MeetingInvitationRequest"];
type Proposal = components["schemas"]["MeetingProposalRequest"];

export type BookingDraft = Readonly<{
  contactId: string;
  contactName?: string;
  attendee: string;
  subject: string;
  location: string;
  description: string;
  duration: number;
  video: boolean;
  provider: string;
}>;
export type BookingEdit = Partial<
  Pick<
    BookingDraft,
    "attendee" | "subject" | "location" | "description" | "video"
  >
> & { contact?: { id: string; name: string } };

export function videoLabel(provider: string): MessageKey {
  if (provider === "gcal") return "scheduling.videoGoogle";
  return provider === "graphcal"
    ? "scheduling.videoTeams"
    : "scheduling.videoGeneric";
}

const NOTES: Record<BookingMode, MessageKey> = {
  propose: "scheduling.proposeNote",
  invite: "scheduling.inviteNote",
  link: "scheduling.linkNote",
};

export function BookingReview({
  mode,
  draft,
  onEdit,
  picks,
  onRemovePick,
  configured,
  zone,
  searchContacts,
}: Readonly<{
  mode: BookingMode;
  draft: BookingDraft;
  onEdit: (edit: BookingEdit) => void;
  picks: readonly BookingSlot[];
  onRemovePick: (slot: BookingSlot) => void;
  configured: boolean;
  zone: string;
  searchContacts: (q: string) => Promise<{ id: string; name: string }[]>;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const ready =
    configured &&
    !!draft.contactId &&
    !!draft.attendee &&
    !!draft.subject.trim();
  const needed = mode === "propose" ? 2 : mode === "invite" ? 1 : 0;
  const cta = (() => {
    if (!configured) return t("scheduling.connectFirst");
    if (picks.length < needed)
      return t(
        mode === "propose" ? "scheduling.pickTwo" : "scheduling.pickOne",
      );
    if (mode === "propose")
      return plural("scheduling.reviewTimes", picks.length, {
        count: formatNumber(picks.length, locale),
      });
    if (mode === "invite")
      return t("scheduling.sendInviteAt", {
        time: formatDateTime(picks[0].start, locale, zone),
      });
    return t("scheduling.createLinkReview");
  })();
  const shared = {
    contact_id: draft.contactId,
    attendee_email: draft.attendee,
    subject: draft.subject,
    location: draft.video ? "" : draft.location,
    description: draft.description,
    video_call: draft.video,
  };
  return (
    <Panel title={t("scheduling.review")}>
      <PanelBody className="book-review">
        <RecordPicker
          label={t("scheduling.contact")}
          selected={
            draft.contactName
              ? { id: draft.contactId, name: draft.contactName }
              : null
          }
          onPick={(contact) => onEdit({ contact })}
          searchTargets={searchContacts}
        />
        <Field label={t("book.attendee")}>
          {(control) => (
            <TextInput
              {...control}
              type="email"
              value={draft.attendee}
              onChange={(e) => onEdit({ attendee: e.target.value })}
            />
          )}
        </Field>
        <Switch
          label={t(videoLabel(draft.provider))}
          hint={t("scheduling.videoHint")}
          checked={draft.video}
          onChange={(video) => onEdit({ video })}
        />
        <Disclosure summary={`${t("scheduling.details")} · ${draft.subject}`}>
          <div className="book-form">
            <Field label={t("scheduling.subject")}>
              {(control) => (
                <TextInput
                  {...control}
                  value={draft.subject}
                  onChange={(e) => onEdit({ subject: e.target.value })}
                />
              )}
            </Field>
            {!draft.video && (
              <Field
                label={t("scheduling.location")}
                hint={t("scheduling.locationHelp")}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    value={draft.location}
                    onChange={(e) => onEdit({ location: e.target.value })}
                  />
                )}
              </Field>
            )}
          </div>
        </Disclosure>
        {mode === "link" ? (
          <GuestPicks name={draft.contactName} />
        ) : (
          <PickTray
            mode={mode}
            picks={picks}
            zone={zone}
            onRemove={onRemovePick}
          />
        )}
        <Field label={t("scheduling.agenda")}>
          {(control) => (
            <Textarea
              {...control}
              value={draft.description}
              onChange={(e) => onEdit({ description: e.target.value })}
            />
          )}
        </Field>
        {mode === "invite" ? (
          <InviteSend
            label={cta}
            disabled={!ready || picks.length < 1}
            request={picks[0] ? { ...shared, ...picks[0] } : undefined}
          />
        ) : (
          <ProposalSend
            label={cta}
            disabled={!ready || picks.length < needed}
            request={{
              ...shared,
              duration_minutes: draft.duration,
              options: mode === "link" ? [] : [...picks],
            }}
            zone={zone}
          />
        )}
        <p className="t-caption">{t(NOTES[mode])}</p>
      </PanelBody>
    </Panel>
  );
}

// Where the other modes list the times picked, a personal link says who picks,
// in the same place and at the same height.
function GuestPicks({ name }: Readonly<{ name?: string }>) {
  const t = useT();
  return (
    <section className="book-picks">
      <span className="t-name">
        {t("scheduling.guestPicks", {
          name: name?.trim() || t("scheduling.guest"),
        })}
      </span>
      <p className="t-caption">{t("scheduling.guestPicksOne")}</p>
    </section>
  );
}

function PickTray({
  mode,
  picks,
  zone,
  onRemove,
}: Readonly<{
  mode: Exclude<BookingMode, "link">;
  picks: readonly BookingSlot[];
  zone: string;
  onRemove: (slot: BookingSlot) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <section
      className="book-picks"
      aria-label={t(
        mode === "invite" ? "scheduling.agreedTime" : "scheduling.timesToOffer",
      )}
    >
      <div className="book-picks-head">
        <span className="t-name">
          {t(
            mode === "invite"
              ? "scheduling.agreedTime"
              : "scheduling.timesToOffer",
          )}
        </span>
        {mode === "propose" && (
          <span className="t-caption">
            {plural("scheduling.pickedOf", picks.length, {
              count: formatNumber(picks.length, locale),
            })}
          </span>
        )}
      </div>
      {picks.length === 0 ? (
        <p className="t-caption">
          {t(
            mode === "invite"
              ? "scheduling.pickAgreedEmpty"
              : "scheduling.pickOfferEmpty",
          )}
        </p>
      ) : (
        <ul className="book-picks-list">
          {picks.map((pick) => {
            const label = formatDateTime(pick.start, locale, zone);
            return (
              <li key={pick.start}>
                <span>{label}</span>
                <IconAction
                  label={t("scheduling.removeTime", { time: label })}
                  icon={<X aria-hidden />}
                  onClick={() => onRemove(pick)}
                />
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function InviteSend({
  label,
  disabled,
  request,
}: Readonly<{ label: string; disabled: boolean; request?: Invitation }>) {
  const intent = useBookingIntent();
  const client = useQueryClient();
  const send = useMutation({
    mutationFn: async (body: Invitation) => {
      const { data, error } = await api.POST("/scheduling/invitations", {
        body,
        params: { header: { "Idempotency-Key": intent(body) } },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async (value, body) => {
      await refreshContactMeetings(client, body.contact_id);
      navigate({ screen: "book", id: `meeting-${value.id}` });
    },
  });
  return (
    <div className="book-form">
      <Button
        variant="primary"
        disabled={disabled || !request}
        pending={send.isPending}
        onClick={() => {
          if (request) send.mutate(request);
        }}
      >
        {label}
      </Button>
      <ErrorLine error={send.error} />
    </div>
  );
}

function ProposalSend({
  label,
  disabled,
  request,
  zone,
}: Readonly<{
  label: string;
  disabled: boolean;
  request: Proposal;
  zone: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const intent = useBookingIntent();
  const client = useQueryClient();
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
    onSuccess: async (_proposal, body) => {
      setReview(true);
      await refreshContactMeetings(client, body.contact_id);
    },
  });
  const current =
    create.data && JSON.stringify(create.variables) === JSON.stringify(request);
  const copy = useClipboardCopy(create.data?.url ?? "", {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  const offered = create.variables ?? request;
  return (
    <div className="book-form">
      <Button
        variant="primary"
        disabled={disabled}
        pending={create.isPending}
        onClick={() => (current ? setReview(true) : create.mutate(request))}
      >
        {label}
      </Button>
      {current && create.data && (
        <div className="book-actions">
          <span className="t-caption">
            {t("scheduling.linkReady", {
              date: formatDateTime(create.data.expires_at, locale, zone),
            })}
          </span>
          <Button variant="link" onClick={copy.copy}>
            {copy.label}
          </Button>
          {copy.notice}
        </div>
      )}
      {!current && create.data && (
        <p className="t-caption">{t("scheduling.linkReplaced")}</p>
      )}
      <ErrorLine error={create.error} />
      {review && create.data && (
        <ComposeModal
          key={create.data.id}
          entityType="contact"
          entityId={offered.contact_id}
          contactId={offered.contact_id}
          recordAddress={offered.attendee_email}
          initialMessage={{
            subject: offered.subject,
            body: proposalEmailBody(
              t,
              { ...offered, url: create.data.url },
              locale,
              zone,
            ),
          }}
          open
          onClose={() => setReview(false)}
        />
      )}
    </div>
  );
}
