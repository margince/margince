// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarPlus } from "lucide-react";
import { Button } from "../../design-system/atoms";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useCalendarConnect } from "./booking-calendar-state";
import { ConnectRefusal } from "./meeting-calendar";

/**
 * What a host without a calendar does first, as the page's lead card. The
 * calendar step is the only one with a verb. Hours are set in the card below,
 * and bookings turn on once the calendar is there.
 */
export function MeetingSetupCard({
  hoursChosen,
  hasLink,
}: Readonly<{ hoursChosen: boolean; hasLink: boolean }>) {
  const t = useT();
  const calendar = useCalendarConnect();
  return (
    <Panel title={t("scheduling.setup.title")}>
      <PanelBody>
        <PanelIntro>{t("scheduling.setup.intro")}</PanelIntro>
        <Checklist>
          <ChecklistStep
            state="current"
            title={t("scheduling.setup.step1")}
            caption={t("scheduling.setup.step1Help")}
            action={
              <Button
                variant="primary"
                pending={calendar.connect.isPending}
                onClick={() => calendar.start(calendar.target)}
              >
                <CalendarPlus aria-hidden />
                {t("scheduling.connectCalendar")}
              </Button>
            }
          />
          <ChecklistStep
            state={hoursChosen ? "done" : "open"}
            title={t("scheduling.setup.step2")}
            caption={t(
              hoursChosen
                ? "scheduling.setup.step2Done"
                : "scheduling.setup.step2Help",
            )}
          />
          <ChecklistStep
            state="open"
            title={t("scheduling.setup.step3")}
            caption={hasLink ? t("scheduling.setup.step3Help") : undefined}
          />
        </Checklist>
        <ConnectRefusal connect={calendar.connect} />
      </PanelBody>
    </Panel>
  );
}
