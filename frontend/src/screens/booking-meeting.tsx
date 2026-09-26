import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { BookingBack, BookingFooter } from "./booking-common";
import { BookingReschedule } from "./booking-reschedule";
import { problemCodeOf, QueryGate, throwProblem } from "./common";
import { ContactMeetingBrief } from "./meetingbrief/drawer";

type Invitation = components["schemas"]["MeetingInvitation"];
type Change = components["schemas"]["MeetingInvitationChange"];
export function BookingMeetingScreen({
  id,
  token,
}: Readonly<{ id?: string; token?: string }>) {
  const t = useT();
  const { locale } = useLocale();
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
      return status === "pending" ||
        status === "rescheduling" ||
        status === "canceling"
        ? 3000
        : false;
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
  const labels: Record<Invitation["status"], string> = {
    pending: t("scheduling.pending"),
    confirmed: t("scheduling.confirmed"),
    needs_attention: t("scheduling.needs_attention"),
    rescheduling: t("scheduling.rescheduling"),
    canceling: t("scheduling.canceling"),
    canceled: t("scheduling.canceled"),
  };
  return (
    <div className={token ? "book-guest-page" : "book-page"}>
      <div className="book-guest-column">
        {!token && <BookingBack />}
        <QueryGate pendingLabel={t("common.loading")} query={query}>
          {(meeting) => (
            <Panel tone={meeting.status === "confirmed" ? "success" : "info"}>
              <PanelBody>
                <div role="status">
                  <Heading as="h1" size="large">
                    {labels[meeting.status]}
                  </Heading>
                </div>
                <Heading as="h2" size="medium">
                  {meeting.subject}
                </Heading>
                <p>
                  {formatDateTime(meeting.start, locale, viewerZone())} –{" "}
                  {formatDateTime(meeting.end, locale, viewerZone())}
                </p>
                <p>{meeting.location}</p>
                {!token &&
                  meeting.status === "confirmed" &&
                  meeting.reminder_status &&
                  meeting.reminder_status !== "off" && (
                    <p className="t-caption">
                      {t(`scheduling.reminder.${meeting.reminder_status}`)}
                    </p>
                  )}
                <p className="t-caption">{t("scheduling.deliveryHelp")}</p>
                <div className="book-actions">
                  {!token && meeting.status === "confirmed" && (
                    <Button onClick={() => setBrief(true)}>
                      {t("scheduling.prepare")}
                    </Button>
                  )}
                  {meeting.calendar_url && !token && (
                    <a
                      href={meeting.calendar_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {t("scheduling.openCalendar")}
                    </a>
                  )}
                  {meeting.status !== "canceled" &&
                    meeting.status !== "canceling" && (
                      <Button
                        variant="danger"
                        disabled={change.isPending}
                        onClick={() => setCancelOpen(true)}
                      >
                        {t("scheduling.cancel")}
                      </Button>
                    )}
                  {meeting.status === "needs_attention" && !token && (
                    <Button
                      disabled={change.isPending}
                      onClick={() =>
                        change.mutate({
                          id,
                          token,
                          body: { action: "retry", version: meeting.version },
                        })
                      }
                    >
                      {t("scheduling.retry")}
                    </Button>
                  )}
                </div>
                {meeting.status === "confirmed" && (
                  <>
                    <Button onClick={() => setReschedule(!reschedule)}>
                      {t("scheduling.reschedule")}
                    </Button>
                    {reschedule && (
                      <BookingReschedule
                        id={id}
                        token={token}
                        pending={change.isPending}
                        onSelect={(slot) =>
                          change.mutate({
                            id,
                            token,
                            body: {
                              action: "reschedule",
                              version: meeting.version,
                              ...slot,
                            },
                          })
                        }
                      />
                    )}
                  </>
                )}
                {meeting.status !== "canceled" &&
                  meeting.status !== "canceling" && (
                    <ConfirmModal
                      open={cancelOpen}
                      onClose={() => setCancelOpen(false)}
                      title={t("scheduling.cancel")}
                      confirmLabel={t("scheduling.cancel")}
                      confirmVariant="danger"
                      pending={change.isPending}
                      onConfirm={() =>
                        change.mutate({
                          id,
                          token,
                          body: { action: "cancel", version: meeting.version },
                        })
                      }
                    >
                      <p>{t("scheduling.cancelConfirm")}</p>
                      {changeError}
                    </ConfirmModal>
                  )}
                {meeting.status === "needs_attention" && (
                  <p>
                    {t(
                      token
                        ? "scheduling.guestRecovery"
                        : "scheduling.hostRecovery",
                    )}
                  </p>
                )}
                {changeError}
              </PanelBody>
            </Panel>
          )}
        </QueryGate>
        {token && <BookingFooter />}
        {!token && id && brief && (
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
