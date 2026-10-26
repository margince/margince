// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { OAuthOutcomeNote } from "../../components/connectors.notices";
import { PendingBody } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useConnectAccount } from "../../lib/connect-account";
import { useConnectors } from "../../lib/connectors";
import { problemMessageOf } from "../../lib/problem";
import { useRoute } from "../../lib/router";
import { useSchedulingProfile } from "../../lib/scheduling-profile-query";
import { useMe } from "../../lib/session";
import { needsAttention, useCalendarStanding } from "./booking-calendar-state";
import { AvailabilityCard } from "./meeting-availability";
import { MeetingCalendarCard } from "./meeting-calendar";
import { GuestViewCard } from "./meeting-guest-view";
import { BookingLinksCard } from "./meeting-links";
import { MeetingSetupCard } from "./meeting-setup";
import { useWorkingHours } from "./working-hours";

type Profile = components["schemas"]["SchedulingProfile"];

export function MeetingSettings() {
  const t = useT();
  const me = useMe();
  const canBook = useCanWrite("activity", "create");
  // Until the grant is known, neither page: the hours-only card would flash
  // and take a first edit with it when the full page replaced it.
  if (me.isPending) return <PendingBody label={t("scheduling.settings")} />;
  if (!canBook)
    return (
      <>
        <Callout kind="standing" title={t("scheduling.bookNeedsGrant")} />
        <AvailabilityCard hoursOnly />
      </>
    );
  return <MeetingPreferences />;
}

function MeetingPreferences() {
  const t = useT();
  const profile = useSchedulingProfile();
  return (
    <>
      <MeetingsLanding />
      <QueryGate pendingLabel={t("common.loading")} query={profile}>
        {(saved) => <MeetingCards profile={saved} />}
      </QueryGate>
    </>
  );
}

// The consent callback for a calendar connected from this page lands on
// #/settings/meetings/{outcome}/{provider}.
function MeetingsLanding() {
  const t = useT();
  const route = useRoute();
  const connections = useConnectors();
  const connect = useConnectAccount();
  return (
    <OAuthOutcomeNote
      outcome={route.id2}
      provider={route.id3}
      connections={connections.data?.data}
      connectCalendar={{
        onConnect: (calendar) =>
          connect.mutate({ provider: calendar, returnTo: "meetings" }),
        pending: connect.isPending,
        error: connect.isError ? problemMessageOf(connect.error, t) : null,
      }}
    />
  );
}

// In board order. The setup card leads only while no calendar is connected;
// until the roster answers, nobody is told to connect what they may have.
function MeetingCards({ profile }: Readonly<{ profile: Profile }>) {
  const { standing } = useCalendarStanding(profile);
  const hours = useWorkingHours();
  const setup = standing === "none";
  return (
    <>
      {setup && (
        <MeetingSetupCard
          hoursChosen={hours.data?.chosen ?? false}
          hasLink={!!profile.slug}
        />
      )}
      <BookingLinksCard profile={profile} />
      <MeetingCalendarCard profile={profile} />
      <AvailabilityCard
        profile={profile}
        attention={needsAttention(profile, standing)}
      />
      <GuestViewCard profile={profile} />
    </>
  );
}
