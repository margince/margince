import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import { useRowSource } from "./worklist.noticesource";
import { RowName } from "./worklist.rowname";

function SourceExample({
  missing = false,
  unavailable = false,
}: {
  missing?: boolean;
  unavailable?: boolean;
}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-one": () =>
      missing
        ? jsonResponse(
            { code: "not_found", status: 404, title: "Not found" },
            404,
          )
        : jsonResponse({
            id: "task-one",
            kind: "task",
            subject: "Send revised scope",
            body: "Include the agreed acceptance criteria.",
            occurred_at: "2026-07-06T09:00:00Z",
            content_state: "available",
            links: [],
          }),
  });
  const source = useRowSource(
    row(
      unavailable
        ? {
            source: "conversation_claim",
            subject: { type: "activity", id: "private-source" },
          }
        : { source: "task", id: "task-one" },
    ),
  );
  return (
    <>
      <RowName
        title="Send revised scope"
        open={source.open}
        unavailable={source.unavailable}
      />
      {source.drawer}
    </>
  );
}
const meta: Meta<typeof SourceExample> = {
  title: "Records/Worklist source context",
  component: SourceExample,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof SourceExample>;
export const TaskContext: Story = {
  args: { missing: false },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Send revised scope" }),
    );
  },
};
export const TaskContextDark: Story = {
  ...TaskContext,
  globals: { theme: "dark" },
};
export const MissingSource: Story = { ...TaskContext, args: { missing: true } };
export const MissingSourceDark: Story = {
  ...MissingSource,
  globals: { theme: "dark" },
};

export const UnavailableSource: Story = { args: { unavailable: true } };
export const UnavailableSourceDark: Story = {
  ...UnavailableSource,
  globals: { theme: "dark" },
};
