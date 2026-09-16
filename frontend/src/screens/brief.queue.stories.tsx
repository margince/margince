// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { announceAddressChanged } from "../app/router";
import { readingsDay, taskRow } from "./brief.fixtures";
import { BriefQueue } from "./brief.queue";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

function QueueFrame() {
  useEffect(() => {
    const before = window.location.hash;
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /worklist": () =>
        jsonResponse(
          readingsDay(
            {},
            [
              {
                ...taskRow("buyer", "Send the promised rollout comparison"),
                contact: {
                  id: "contact-sonya",
                  label: "Sonya Beck",
                  touch: {
                    last_inbound_at: "2026-09-03T16:46:00Z",
                    last_outbound_at: "2026-08-28T09:12:00Z",
                  },
                },
              },
            ],
            [
              {
                category: "tasks",
                considered: 1,
                shown: 1,
                more_available: false,
              },
            ],
            { in_play: 1 },
          ),
        ),
      "GET /worklist/handled": () =>
        jsonResponse({
          as_of: "2026-09-13T08:00:00Z",
          receipts: [],
          truncated: false,
        }),
    });
    window.location.hash = "#/home?queue=1";
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
  }, []);
  return (
    <StoryProviders>
      <BriefQueue />
    </StoryProviders>
  );
}

const meta: Meta<typeof BriefQueue> = {
  title: "Shell/Home work queue",
  component: BriefQueue,
  render: () => <QueueFrame />,
};
export default meta;
type Story = StoryObj<typeof BriefQueue>;
export const Desktop: Story = {};
export const Phone: Story = { tags: ["uat-phone"] };
