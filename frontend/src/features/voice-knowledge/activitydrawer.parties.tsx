// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Activity = components["schemas"]["Activity"];
type ActivityLink = components["schemas"]["ActivityLink"];
type MeetingGuest = components["schemas"]["MeetingGuest"];
type MeetingResponse = components["schemas"]["MeetingResponse"];

export function AttendeesLine({
  attendees,
  guests,
  answers,
  leftOut = 0,
}: Readonly<{
  attendees: readonly ActivityLink[];
  guests: readonly MeetingGuest[];
  answers: ReadonlyMap<string, MeetingResponse>;
  leftOut?: number;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (attendees.length + guests.length === 0) {
    return null;
  }
  return (
    <PartyLine label={t("activity.detail.attendees")}>
      {attendees.map((link, index) => (
        <Fragment key={link.entity_id}>
          {index > 0 && ", "}
          <EntityRef kind="contact" id={link.entity_id} newTab />
          <ResponseBadge response={answers.get(`contact:${link.entity_id}`)} />
        </Fragment>
      ))}
      {guests.map((guest, index) => (
        <Fragment key={guest.contact_id ?? guest.address}>
          {attendees.length + index > 0 && ", "}
          {guest.contact_id ? (
            <EntityRef kind="contact" id={guest.contact_id} newTab />
          ) : (
            <span title={guest.address}>
              {guest.display_name || guest.address}
            </span>
          )}
          <ResponseBadge response={guest.response} />
        </Fragment>
      ))}
      {leftOut > 0 && (
        <>
          {" "}
          {plural("activity.detail.guestsLeftOut", leftOut, {
            count: formatNumber(leftOut, locale),
          })}
        </>
      )}
    </PartyLine>
  );
}

// An accepted answer shows nothing: being on the list already says it.
export function ResponseBadge({
  response,
}: Readonly<{ response: MeetingResponse | undefined }>) {
  const t = useT();
  switch (response) {
    case "declined":
      return (
        <>
          {" "}
          <Badge tone="danger">{t("activity.detail.response.declined")}</Badge>
        </>
      );
    case "tentative":
      return (
        <>
          {" "}
          <Badge tone="warning">
            {t("activity.detail.response.tentative")}
          </Badge>
        </>
      );
    case "needs_action":
      return (
        <>
          {" "}
          <Badge>{t("activity.detail.response.needsAction")}</Badge>
        </>
      );
    default:
      return null;
  }
}

export function answersOf(activity: Activity): Map<string, MeetingResponse> {
  const answers = new Map<string, MeetingResponse>();
  for (const answer of activity.attendee_responses ?? []) {
    if (answer.user_id) {
      answers.set(`user:${answer.user_id}`, answer.response);
    } else if (answer.contact_id) {
      answers.set(`contact:${answer.contact_id}`, answer.response);
    }
  }
  return answers;
}

export function colleaguesOf(activity: Activity): string[] {
  const seats = [activity.host_user_id, ...(activity.colleague_ids ?? [])];
  return [...new Set(seats.filter((id): id is string => Boolean(id)))];
}

export function PartyLine({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">{label}</span>
      {children}
    </p>
  );
}
