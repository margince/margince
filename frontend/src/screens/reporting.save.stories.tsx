import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Analytics/Save report" };
export default meta;
type Story = StoryObj;
function Preview() {
  const [open, setOpen] = useState(true);
  return open ? (
    <SaveReportingDialog
      selection={reportingStoryEvaluation.selection}
      onClose={() => setOpen(false)}
      onSaved={() => setOpen(false)}
    />
  ) : (
    <p>Dialog closed</p>
  );
}
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("dialog");
  },
};

/** Customise open overflows the dialog at a desktop height: the field stack
 *  scrolls and every row keeps its own height, the toggle included. */
export const Customize: Story = {
  ...Default,
  play: async () => {
    const dialog = await screen.findByRole("dialog");
    const toggle = await screen.findByRole("button", {
      name: "Customize report",
    });
    await userEvent.click(toggle);
    await screen.findByRole("group", { name: "Metrics" });
    const stack = dialog.querySelector<HTMLElement>(":scope > .form-stack");
    if (!stack) throw new Error("The dialog drew no field stack.");
    await waitFor(() =>
      expect(stack.scrollHeight).toBeGreaterThan(stack.clientHeight),
    );
    await expect(toggle.offsetHeight).toBeGreaterThanOrEqual(
      toggle.scrollHeight,
    );
  },
};
