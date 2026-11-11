import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../testing/story-utils";
import { TimelineBody } from "./timelinetext";

export default {
  title: "Components/Text and data display/Timeline body",
  component: TimelineBody,
} satisfies Meta<typeof TimelineBody>;
export const OriginalTask: StoryObj<typeof TimelineBody> = {
  render: () => (
    <StoryProviders locale="de">
      <TimelineBody
        body="Bitte den Vertrag prüfen."
        entry={{
          id: "task-original",
          kind: "task",
          language: "de",
          sourceVersion: 1,
        }}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("Bitte den Vertrag prüfen."),
    ).toBeVisible();
    await expect(
      within(canvasElement).queryByRole("button", {
        name: "In meiner Sprache anzeigen",
      }),
    ).not.toBeInTheDocument();
  },
};
