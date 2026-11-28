// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { activityTimeline } from "../../design-system/activitytimeline";
import { TimelineList } from "../../design-system/timeline";
import { browserZone } from "../../format/timezone";
import { vi } from "../../i18n/vi";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReceiptNote } from "./worklist.receiptnote";

const note: components["schemas"]["Activity"] = {
  id: "01a05500-0000-7000-8000-0000000000a1",
  kind: "note",
  subject: "Risk assessment",
  body: "No reply in three weeks.",
  language: "en",
  version: 4,
  occurred_at: "2026-10-20T10:00:00Z",
  occurred_precision: "instant",
  links: [],
  source: "manual",
  captured_by: "agent:passport",
  is_done: false,
  offers_email: false,
  created_at: "2026-10-20T10:00:00Z",
  updated_at: "2026-10-20T10:00:00Z",
  recorded_by_agent: true,
  created_by: {
    actor_type: "agent",
    actor_id: "agent:passport",
    on_behalf_of_name: "Anna Weber",
  },
};

export default {
  title: "Worklist/Change history/AI-written note",
  component: ReceiptNote,
} satisfies Meta<typeof ReceiptNote>;

export const VietnameseReader: StoryObj<typeof ReceiptNote> = {
  args: { activityId: note.id, children: note.subject },
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({ allow: { activity: ["read"] } }),
          reader_rendering_available: true,
        }),
      [`GET /activities/${note.id}`]: () => jsonResponse(note),
      [`POST /activities/${note.id}/reader-rendering`]: () =>
        jsonResponse({
          activity_id: note.id,
          source_version: 4,
          source_language: "en",
          target_language: "vi",
          subject: "Đánh giá rủi ro",
          body: "Chưa có hồi âm trong ba tuần.",
          unchanged: false,
          model_used: "fixture",
          generated_at: note.occurred_at,
        }),
    });
    return (
      <StoryProviders locale="vi">
        <section aria-label="Worklist">
          <ReceiptNote activityId={note.id}>{note.subject}</ReceiptNote>
        </section>
        <section aria-label="History">
          <TimelineList
            entries={activityTimeline([note])}
            zone={browserZone()}
          />
        </section>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    for (const region of ["Worklist", "History"]) {
      const row = within(canvas.getByRole("region", { name: region }));
      await expect(
        await row.findByText(
          vi["trust.agentUnnamedFor"].replace("{name}", "Anna Weber"),
        ),
      ).toBeVisible();
      await expect(
        row.getByRole("button", { name: vi["readerRendering.show"] }),
      ).toBeVisible();
    }
    const worklist = within(canvas.getByRole("region", { name: "Worklist" }));
    await userEvent.click(
      worklist.getByRole("button", { name: vi["readerRendering.show"] }),
    );
    await expect(await worklist.findByText("Đánh giá rủi ro")).toBeVisible();
    await expect(worklist.getByText(note.subject ?? "")).toBeVisible();
  },
};
