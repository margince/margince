// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { VCardImport } from "./vcard-import";

// The dialog is the surface worth reviewing, and it only exists once opened —
// so every story here presses the button and leaves it open. What differs
// between them is the REPORT, which is the part that has to survive being read
// by somebody checking an import against a stack of cards on their desk.

const meta: Meta<typeof VCardImport> = {
  title: "Patterns/vCard import",
  component: VCardImport,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VCardImport>;

const ROUTE = "POST /people/vcard-import";

async function openDialog(canvasElement: HTMLElement) {
  const canvas = within(canvasElement.ownerDocument.body);
  await userEvent.click(await canvas.findByTestId("vcard-import"));
  return canvas;
}

/** The dialog before anything is chosen: what the file is, and why these are
 * written straight in rather than queued. */
export const Empty: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders locale="de">
        <VCardImport />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await openDialog(canvasElement);
    await expect(await canvas.findByTestId("vcard-import-file")).toBeVisible();
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
              person_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
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
        <VCardImport />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await openDialog(canvasElement);
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
    await expect(
      await canvas.findByTestId("vcard-import-report"),
    ).toBeVisible();
  },
};

/** A card the parser could not read fails the WHOLE request rather than being
 * skipped, because an import that quietly drops a person is worse than one
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
        <VCardImport />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = await openDialog(canvasElement);
    const holder = await canvas.findByTestId("vcard-import-file");
    const field = holder.querySelector("input[type=file]");
    if (field instanceof HTMLInputElement) {
      await userEvent.upload(
        field,
        new File(["nonsense"], "kaputt.vcf", { type: "text/vcard" }),
      );
    }
    await expect(await canvas.findByTestId("vcard-import-error")).toBeVisible();
  },
};
