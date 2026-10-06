// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { VCardImportPage } from "./vcard-import";

// What differs between these is the REPORT, which is the part that has to
// survive being read by somebody checking an import against a stack of cards
// on their desk.

const meta: Meta<typeof VCardImportPage> = {
  title: "Patterns/vCard import",
  component: VCardImportPage,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof VCardImportPage>;

const ROUTE = "POST /contacts/vcard-import";

async function arrive(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await canvas.findByRole("heading", { level: 1 });
  return canvas;
}

/** The page before anything is chosen: what the file is, and why these are
 * written straight in rather than queued. */
export const Empty: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders locale="de">
        <VCardImportPage />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await arrive(canvasElement);
    // The page ARRIVES (enter.css), so the dropzone is in the DOM a frame
    // before it is visible.
    const dropzone = await canvas.findByTestId("vcard-import-file");
    await waitFor(() => expect(dropzone).toBeVisible());
  },
};

/** A mixed file, which is the ordinary case: some cards land, some fill gaps
 * in a record that already existed, one resembles somebody and was written
 * nowhere, one carried no name at all. */
export const MixedReport: Story = {
  render: () => {
    installFetchStub({
      [ROUTE]: () =>
        jsonResponse({
          results: [
            { index: 0, full_name: "Ada Lovelace", outcome: "created" },
            { index: 1, full_name: "Grace Hopper", outcome: "updated" },
            {
              index: 2,
              full_name: "Alan Turing",
              outcome: "needs_review",
              contact_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
            },
            {
              index: 3,
              full_name: "(kein Name)",
              outcome: "skipped",
              reason: "Die Karte trug keinen brauchbaren Namen.",
            },
          ],
        }),
    });
    return (
      <StoryProviders locale="de">
        <VCardImportPage />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await arrive(canvasElement);
    const input = (await canvas.findByTestId(
      "vcard-import-file",
    )) as HTMLElement;
    const field = input.querySelector("input[type=file]");
    if (field instanceof HTMLInputElement) {
      await userEvent.upload(
        field,
        new File(["BEGIN:VCARD\nEND:VCARD"], "karten.vcf", {
          type: "text/vcard",
        }),
      );
    }
    // The settled report is what this story asserts, not the arriving node.
    const report = await canvas.findByTestId("vcard-import-report");
    await waitFor(() => expect(report).toBeVisible());
  },
};

/** A card the parser could not read fails the WHOLE request rather than being
 * skipped, because an import that quietly drops a contact is worse than one
 * that refuses. */
export const Refused: Story = {
  render: () => {
    installFetchStub({
      [ROUTE]: () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail: "Karte 2 konnte nicht gelesen werden.",
            status: 422,
          },
          422,
        ),
    });
    return (
      <StoryProviders locale="de">
        <VCardImportPage />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await arrive(canvasElement);
    const holder = await canvas.findByTestId("vcard-import-file");
    const field = holder.querySelector("input[type=file]");
    if (field instanceof HTMLInputElement) {
      await userEvent.upload(
        field,
        new File(["nonsense"], "kaputt.vcf", { type: "text/vcard" }),
      );
    }
    // Same arrival as the report above.
    const error = await canvas.findByTestId("vcard-import-error");
    await waitFor(() => expect(error).toBeVisible());
  },
};
