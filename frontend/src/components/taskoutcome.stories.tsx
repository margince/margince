import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskOutcomeForm } from "./taskoutcome";

const task: components["schemas"]["Activity"] = {
  id: "01a05500-0000-7000-8000-0000000000a1",
  kind: "task",
  subject: "Follow up",
  occurred_at: "2026-10-01T09:00:00Z",
  occurred_precision: "instant",
  source: "manual",
  captured_by: "human:demo",
  created_at: "2026-10-01T09:00:00Z",
  updated_at: "2026-10-01T09:00:00Z",
  offers_email: false,
  is_done: false,
  version: 1,
};
const meta: Meta<typeof TaskOutcomeForm> = {
  title: "Patterns/Task outcome",
  component: TaskOutcomeForm,
};
export default meta;
type Story = StoryObj<typeof meta>;
function draw(links: components["schemas"]["Activity"]["links"]) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create", "update"] }),
    "GET /installation/settings": () =>
      jsonResponse({ max_upload_bytes: 10000000 }),
  });
  return (
    <StoryProviders>
      <TaskOutcomeForm task={{ ...task, links }} onLogged={() => {}} />
    </StoryProviders>
  );
}
export const ChooseRecord: Story = {
  render: () => draw([]),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("searchbox", {
        name: "Which record is this activity for?",
      }),
    ).toBeVisible();
  },
};
export const ContactOutcome: Story = {
  render: () =>
    draw([
      {
        entity_type: "contact",
        entity_id: "01a05500-0000-7000-8000-0000000000b1",
      },
    ]),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("checkbox", { name: "Mark task done" }),
    ).toBeChecked();
    await expect(
      await within(canvasElement).findByText(
        "Drop files here, or click to choose them",
      ),
    ).toBeVisible();
  },
};
export const ContactOutcomeDark: Story = {
  ...ContactOutcome,
  globals: { theme: "dark" },
};
