// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { render } from "@testing-library/react";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { READS_RECORDS } from "../../testing/mefixture";
import { SourceHost } from "../../testing/sourcehost";
import { meRoute, StoryProviders } from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";

export const taskAllow = (verb: "update" | "create") => ({
  ...READS_RECORDS,
  activity: ["read", verb] as const,
});
export const taskMe = (verb: "update" | "create") =>
  meRoute(taskAllow(verb), { seat: "full" });

// The reader's zone is named, so the due day and hour do not move with the
// machine the suite runs on.
export const READER_ZONE = "Europe/Berlin";

export function openTask() {
  return render(
    <StoryProviders>
      <SourceHost>
        <ViewerZoneProvider zone={READER_ZONE}>
          <TaskDetailModal
            activityId="task-1"
            readOnly={false}
            onClose={() => {}}
            update={
              {
                mutate: () => {},
                isPending: false,
              } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
            }
          />
        </ViewerZoneProvider>
      </SourceHost>
    </StoryProviders>,
  );
}
