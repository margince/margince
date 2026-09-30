import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { ActionRow } from "../design-system/actionrow";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDayFull, formatTimeRange } from "../format/meetingtime";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { BookingFooter } from "./booking-common";
import {
  DeliveryCard,
  MeetingFacts,
  openCalendarLabel,
} from "./booking-meeting-parts";
import { BookingReschedule } from "./booking-reschedule";
import { problemCodeOf, QueryGate, throwProblem } from "./common";
import { ContactMeetingBrief } from "./meetingbrief/drawer";
import "./booking-meeting.css";

type Invitation = components["schemas"]["MeetingInvitation"];
type Change = components["schemas"]["MeetingInvitationChange"];
type Status = Invitation["status"];

const STATUS_TONE: Readonly<
  Record<Status, "info" | "success" | "warning" | "default">
> = {
  pending: "info",
  confirmed: "success",
  needs_attention: "warning",
  rescheduling: "info",
  canceling: "info",
  canceled: "default",
};
const IN_FLIGHT: ReadonlySet<Status> = new Set([
  "pending",
  "rescheduling",
  "canceling",
]);

// The host reaches a meeting from a contact, the worklist or a link in their
// calendar, so "back" is wherever they were; a page opened cold has no history
// to return to and lands on home instead.
function leaveMeeting() {
  if (globalThis.history.length > 1) globalThis.history.back();
  else navigate({ screen: "home" });
}

export function BookingMeetingScreen({
  id,
  token,
}: Readonly<{ id?: string; token?: string }>) {
  const t = useT();
  const client = useQueryClient();
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reschedule, setReschedule] = useState(false);
  const [brief, setBrief] = useState(false);
  const queryKey = ["meeting-invitation", id, token];
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      const result = token
        ? await api.GET("/public/meeting/{token}", {
            params: { path: { token } },
          })
        : await api.GET("/scheduling/invitations/{id}", {
            params: { path: { id: id ?? "" } },
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && IN_FLIGHT.has(status) ? 3000 : false;
    },
  });
  const change = useMutation({
    mutationFn: async (input: {
      id?: string;
      token?: string;
      body: Change;
    }) => {
      const result = input.token
        ? await api.PATCH("/public/meeting/{token}", {
            params: { path: { token: input.token } },
            body: input.body,
          })
        : await api.PATCH("/scheduling/invitations/{id}", {
            params: { path: { id: input.id ?? "" } },
            body: input.body,
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    onError: async () => {
      await client.invalidateQueries({ queryKey });
    },
    onSuccess: (value) => {
      client.setQueryData(queryKey, value);
      setCancelOpen(false);
      setReschedule(false);
    },
  });
  const changeError =
    problemCodeOf(change.error) === "conflict" ? (
      <ErrorLine>{t("scheduling.meetingChanged")}</ErrorLine>
    ) : (
      <ErrorLine error={change.error} />
    );
  const host = !token;
  return (
    <div className={host ? "book-page" : "book-guest-page"}>
      <div className="book-guest-column">
        {host && (
          <div>
            <Button variant="link" onClick={leaveMeeting}>
              <ChevronLeft aria-hidden="true" />
              {t("scheduling.backLink")}
            </Button>
          </div>
        )}
        <QueryGate pendingLabel={t("common.loading")} query={query}>
          {(meeting) => {
            const cancelable =
              meeting.status !== "canceled" && meeting.status !== "canceling";
            const send = (body: Change) => change.mutate({ id, token, body });
            return (
              <div className={host ? "bookmeet bookmeet-host" : "bookmeet"}>
                <Panel>
                  <PanelBody className="bookmeet-main">
                    <MeetingHead meeting={meeting} />
                    <AttentionNotice
                      meeting={meeting}
                      host={host}
                      pending={change.isPending}
                      onRetry={() =>
                        send({ action: "retry", version: meeting.version })
                      }
                    />
                    <MeetingFacts meeting={meeting} host={host} />
                    <ActionRow>
                      {host && meeting.status === "confirmed" && (
                        <Button variant="ai" onClick={() => setBrief(true)}>
                          {t("scheduling.prepare")}
                        </Button>
                      )}
                      {meeting.status === "confirmed" && (
                        <Button
                          aria-expanded={reschedule}
                          onClick={() => setReschedule(!reschedule)}
                        >
                          {t("scheduling.reschedule")}
                        </Button>
                      )}
                      {host && meeting.calendar_url && (
                        <OffsiteLink href={meeting.calendar_url}>
                          {openCalendarLabel(t, meeting.provider)}
                        </OffsiteLink>
                      )}
                      {cancelable && (
                        <Button
                          variant="danger"
                          disabled={change.isPending}
                          onClick={() => setCancelOpen(true)}
                        >
                          {t("scheduling.cancel")}
                        </Button>
                      )}
                    </ActionRow>
                    {meeting.status === "confirmed" && reschedule && (
                      <BookingReschedule
                        id={id}
                        token={token}
                        pending={change.isPending}
                        onSelect={(slot) =>
                          send({
                            action: "reschedule",
                            version: meeting.version,
                            ...slot,
                          })
                        }
                      />
                    )}
                    {changeError}
                  </PanelBody>
                </Panel>
                {host && <DeliveryCard status={meeting.status} />}
                {cancelable && (
                  <ConfirmModal
                    open={cancelOpen}
                    onClose={() => setCancelOpen(false)}
                    title={t("scheduling.cancel")}
                    confirmLabel={t("scheduling.cancel")}
                    confirmVariant="danger"
                    pending={change.isPending}
                    onConfirm={() =>
                      send({ action: "cancel", version: meeting.version })
                    }
                  >
                    <p>{t("scheduling.cancelConfirm")}</p>
                    {changeError}
                  </ConfirmModal>
                )}
              </div>
            );
          }}
        </QueryGate>
        {!host && <BookingFooter />}
        {host && id && brief && (
          <ContactMeetingBrief
            activityId={id}
            open
            onClose={() => setBrief(false)}
          />
        )}
      </div>
    </div>
  );
}

/** Where the invitation stands, then what the meeting is and when. */
function MeetingHead({ meeting }: Readonly<{ meeting: Invitation }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const labels: Record<Status, string> = {
    pending: t("scheduling.pending"),
    confirmed: t("scheduling.confirmed"),
    needs_attention: t("scheduling.needs_attention"),
    rescheduling: t("scheduling.rescheduling"),
    canceling: t("scheduling.canceling"),
    canceled: t("scheduling.canceled"),
  };
  return (
    <div className="bookmeet-head">
      <div role="status">
        <Badge
          tone={STATUS_TONE[meeting.status]}
          live={IN_FLIGHT.has(meeting.status)}
        >
          {labels[meeting.status]}
        </Badge>
      </div>
      <Heading as="h1" size="large">
        {meeting.subject || t("contact.meetings.untitled")}
      </Heading>
      <p className="t-num">
        {formatDayFull(meeting.start, locale, zone)} ·{" "}
        {formatTimeRange(meeting.start, meeting.end, locale, zone)}
      </p>
    </div>
  );
}

// A calendar that refused the invitation is the one state that asks the host
// for a move, and retrying comes before giving up: it resends the SAME
// invitation, so it cannot double-book the guest.
function AttentionNotice({
  meeting,
  host,
  pending,
  onRetry,
}: Readonly<{
  meeting: Invitation;
  host: boolean;
  pending: boolean;
  onRetry: () => void;
}>) {
  const t = useT();
  if (meeting.status !== "needs_attention") return null;
  if (!host)
    return <Callout tone="warning" title={t("scheduling.guestRecovery")} />;
  return (
    <Callout
      tone="warning"
      title={t("scheduling.attentionTitle")}
      actions={
        <Button variant="primary" disabled={pending} onClick={onRetry}>
          {t("scheduling.retry")}
        </Button>
      }
    >
      <p>{t("scheduling.hostRecovery")}</p>
      <p>{t("scheduling.retryHelp")}</p>
    </Callout>
  );
}
