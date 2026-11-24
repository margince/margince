// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fireEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskCompletionChoice } from "./logactivity.taskcompletion";
import { LogActivityFilesField, type PickedFile } from "./logactivityfiles";

function OutcomeFiles() {
  const [done, setDone] = useState(true);
  const [files, setFiles] = useState<readonly PickedFile[]>([
    {
      id: "pdf",
      file: new File(["Demo outcome"], "Call notes.pdf", {
        type: "application/pdf",
      }),
    },
  ]);
  return (
    <div className="form-stack">
      <LogActivityFilesField files={files} onChange={setFiles} />
      <TaskCompletionChoice
        enabled
        checked={done}
        onChange={setDone}
        pending={false}
      />
    </div>
  );
}
const meta: Meta<typeof LogActivityFilesField> = {
  title: "Patterns/Activity outcome files",
  component: LogActivityFilesField,
};
export default meta;
type Story = StoryObj<typeof meta>;
export const Pdf: Story = {
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("Call notes.pdf"),
    ).toBeVisible();
  },
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["create", "update"] }),
      "GET /installation/settings": () =>
        jsonResponse({ max_upload_bytes: 10000000 }),
    });
    return (
      <StoryProviders>
        <OutcomeFiles />
      </StoryProviders>
    );
  },
};
export const PdfDark: Story = { ...Pdf, globals: { theme: "dark" } };

// A dropped file the server would refuse is named at once, and never held.
export const RefusedAsDropped: Story = {
  render: Pdf.render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // A browser builds a DragEvent only from a real DataTransfer.
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(
      new File(["MZ"], "setup.exe", { type: "application/x-msdownload" }),
    );
    fireEvent(
      await canvas.findByLabelText("Attach files"),
      new DragEvent("drop", { dataTransfer, bubbles: true, cancelable: true }),
    );
    await expect(
      await canvas.findByText(/^setup\.exe was not added\./),
    ).toBeVisible();
  },
};
