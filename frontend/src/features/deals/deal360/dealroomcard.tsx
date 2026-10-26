// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { RoomStateBadge, useDealRoom } from "../../../components/dealroom";
import { Button } from "../../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../../design-system/overviewcard";
import { formatDayMonth, formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useRecordZone } from "../../../lib/recordzone";
import { useRoomAttendance } from "../dealroomaccess.queries";

type DealRoom = components["schemas"]["DealRoom"];

/**
 * The deal's room on its Overview: whether the buyer has one, who has been
 * in and when a buyer last looked. The counts are the room page's own read,
 * so the two never disagree; the link opens the Deal Room tab.
 */
export function DealRoomCard({
  dealId,
  onOpen,
}: Readonly<{ dealId: string; onOpen: () => void }>) {
  const t = useT();
  const read = useDealRoom(dealId);
  const room = read.data?.data?.[0];
  // "No room yet" is an answer, so it waits for the read to give it.
  const answered = read.isSuccess;
  return (
    <OverviewCard
      title={t("room.card.title")}
      action={
        answered ? (
          <Button variant="link" onClick={onOpen}>
            {room ? t("deal.room.open") : t("deal.room.setUp")}
          </Button>
        ) : undefined
      }
    >
      {answered && (
        <OverviewList>
          {room ? (
            <RoomRow room={room} />
          ) : (
            <OverviewRow
              title={t("deal.room.none")}
              sub={t("deal.room.noneSub")}
            />
          )}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

function RoomRow({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const { invited, active, lastSeen, counted } = useRoomAttendance(room.id);
  // Until the seats are read, the room's own name stands in: "Nobody invited"
  // before the read answers would be a claim the page cannot back.
  if (!counted) {
    return (
      <OverviewRow
        title={room.title}
        right={<RoomStateBadge state={room.state} />}
      />
    );
  }
  const sub =
    invited > 0
      ? [
          t("deal.room.signedIn", { active: formatNumber(active, locale) }),
          ...(lastSeen
            ? [
                t("deal.room.lastSeen", {
                  when: formatDayMonth(lastSeen, locale, zone),
                }),
              ]
            : []),
        ].join(" · ")
      : room.title;
  return (
    <OverviewRow
      title={
        invited > 0
          ? t("deal.room.invited", { invited: formatNumber(invited, locale) })
          : t("deal.room.nobodyInvited")
      }
      sub={sub}
      right={<RoomStateBadge state={room.state} />}
    />
  );
}
