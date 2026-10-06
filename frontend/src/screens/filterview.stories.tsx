// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { FilterVocabulary } from "./filterdata";
import { OpenedViewPage } from "./filterview";
import { listsMe } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// `#/filters/<tab>/<viewId>`: a saved view opened to be read first, its
// filter as one sentence above what it selects, and edited in place.
const meta: Meta<typeof OpenedViewPage> = {
  title: "Patterns/Filters and views/Opened view",
  component: OpenedViewPage,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof OpenedViewPage>;
type Canvas = ReturnType<typeof within>;

const VIEW_ID = "01a0f000-0000-7000-8000-0000000000a1";

const VOCABULARY: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "city",
      type: "text",
      operators: ["eq", "neq", "contains"],
      custom: false,
    },
    {
      name: "last_activity_at",
      type: "date",
      operators: ["gt", "lt"],
      custom: false,
    },
  ],
};

const VIEW = {
  id: VIEW_ID,
  owner_id: "00000000-0000-4000-8000-000000000001",
  resource: "contacts",
  name: "Berlin contacts, quiet 45 days",
  shared_scope: "private",
  query: {
    filter: {
      and: [
        { field: "city", op: "eq", value: "Berlin" },
        { field: "last_activity_at", op: "lt", value: { days_ago: 45 } },
      ],
    },
  },
  version: 3,
};

const ROWS = Array.from({ length: 25 }, (_, index) => ({
  id: `p-${index}`,
  full_name: `Contact ${index + 1}`,
  city: "Berlin",
  last_activity_at: "2026-07-01",
}));

const problem = (status: number, code: string, detail: string) =>
  jsonResponse({ title: "Refused", status, code, detail }, status);

type Answer = "view" | "held" | "gone" | "failed" | "conflict" | "readSeat";

function routes(answer: Answer = "view") {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /views/${VIEW_ID}`]: () => {
      if (answer === "held") {
        return new Promise<Response>(() => undefined);
      }
      if (answer === "failed") {
        return problem(503, "unavailable", "The server is not answering.");
      }
      return answer === "gone"
        ? problem(404, "not_found", "Not found.")
        : jsonResponse(VIEW);
    },
    [`PATCH /views/${VIEW_ID}`]: () =>
      answer === "conflict"
        ? problem(409, "version_skew", "The view changed since it was read.")
        : jsonResponse({ ...VIEW, version: 4 }),
    "GET /filters/vocabulary": () => jsonResponse(VOCABULARY),
    "POST /filters/preview": () =>
      answer === "readSeat"
        ? problem(403, "seat_tier_insufficient", "seat tier insufficient")
        : jsonResponse({
            resource: "contact",
            match_count: 214,
            columns: ["id", "full_name", "city", "last_activity_at"],
            rows: ROWS,
            truncated: false,
          }),
  });
}

function opened(answer?: Answer) {
  return () => {
    routes(answer);
    return <OpenedViewPage tab="contacts" viewId={VIEW_ID} />;
  };
}

/** Edit conditions, then the city changed: the view no longer says what it saved. */
async function changeCity(canvas: Canvas) {
  await userEvent.click(
    await canvas.findByRole("button", { name: "Edit conditions" }),
  );
  const city = canvas.getByDisplayValue("Berlin");
  await userEvent.clear(city);
  await userEvent.type(city, "Hamburg");
}

// Until the view and the fields that name its sentence are read, the page
// waits under the name the trail gives it, never as an empty builder.
export const Loading: Story = { render: opened("held") };

// Results first: the filter as one sentence, what it selects open beneath it,
// and nothing emerald, because there is nothing yet to keep.
export const Reading: Story = { render: opened() };

// The rows open where the sentence was; with nothing changed, Done folds them.
export const EditingUnchanged: Story = {
  render: opened(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit conditions" }),
    );
  },
};

// Changed: discard, keep it as a new view, or save it back over this one.
export const EditingChanged: Story = {
  render: opened(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await changeCity(canvas);
    await canvas.findByText("Unsaved changes");
  },
};

// Somebody changed the view since it was opened: the save is refused, and the
// way to see their change is beside the refusal.
export const VersionConflict: Story = {
  render: opened("conflict"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await changeCity(canvas);
    await userEvent.click(canvas.getByRole("button", { name: "Save changes" }));
    await canvas.findByText(
      "This view changed since you opened it. Reload it to see the latest version.",
    );
  },
};

// Deleted, or never there: one heading, one sentence, the way back.
export const Gone: Story = { render: opened("gone") };

// A read that failed is not a view that is gone: why, and the read again.
export const ReadFailed: Story = {
  render: opened("failed"),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", { name: "Reload view" });
  },
};

// The view's own verbs, behind the More at the end of its name's row.
export const MoreOpen: Story = {
  render: opened(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "More for Berlin contacts, quiet 45 days",
      }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Delete view",
    });
  },
};

// A read seat opens its view and is refused the count: the count says so, and
// the reason sits in the results.
export const ReadSeat: Story = {
  render: opened("readSeat"),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("alert");
  },
};
