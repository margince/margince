// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DocumentExtractionPanel } from "./documentextraction";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// What a reading of one attachment offers a human, and what it refuses to
// offer. Every state below is a different answer and the differences are the
// point: a reading still running carries empty lists, a finished reading that
// grounded nothing is a correct answer with a reason, and an omitted field says
// whether the document was silent or merely unclear — one sends a rep to the
// document, the other tells them not to bother.
//
// The amount is shown in MAJOR units against the currency the SAME reading
// grounded. A minor-unit integer under a label reading "Amount" is a hundred
// times the price on the offer somebody signs, so the staged value and its
// currency stand or fall together.

const meta: Meta = {
  title: "Screens/Document extraction",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Extraction = components["schemas"]["AttachmentExtraction"];

const grounded: Extraction["fields"] = [
  {
    field: "amount_minor",
    value: "14850000",
    source_quote: "Gesamtsumme: 148.500,00 €",
    page_or_section: "page 3",
    confidence: "high",
  },
  {
    // The currency the amount above is rendered IN comes from this reading, not
    // from the workspace default: an amount scaled by a guessed currency is a
    // wrong number wearing a right label.
    field: "currency",
    value: "EUR",
    source_quote: "alle Beträge in EUR",
    page_or_section: "page 1",
    confidence: "high",
  },
  {
    field: "expected_close_date",
    value: "2026-11-30",
    source_quote: "Laufzeitbeginn 01.12.2026",
    page_or_section: "§2",
    confidence: "medium",
  },
];

function extraction(over: Partial<Extraction>): Extraction {
  return {
    id: "ex-1",
    status: "done",
    fields: [],
    omitted: [],
    created_at: "2026-08-20T09:00:00Z",
    finished_at: "2026-08-20T09:00:12Z",
    ...over,
  } as Extraction;
}

function Panel({
  reading,
  canAccept = true,
}: Readonly<{ reading: Extraction | null; canAccept?: boolean }>) {
  installFetchStub({
    "GET /attachments/a-1/extraction": () =>
      reading
        ? jsonResponse(reading)
        : jsonResponse(
            { title: "Not found", status: 404, detail: "no reading yet" },
            404,
          ),
  });
  return (
    <StoryProviders>
      <DocumentExtractionPanel attachmentId="a-1" canAccept={canAccept} />
    </StoryProviders>
  );
}

// Nobody has read the file: the panel is the OFFER to read it, not an error.
export const NotReadYet: Story = { render: () => <Panel reading={null} /> };

export const Running: Story = {
  render: () => (
    <Panel reading={extraction({ status: "running", finished_at: null })} />
  ),
};

export const StagedFields: Story = {
  render: () => <Panel reading={extraction({ fields: grounded })} />,
};

// The staged values are there to read but not to accept: a reader without the
// grant sees what the document says and is offered no write.
export const StagedWithoutTheGrant: Story = {
  render: () => (
    <Panel reading={extraction({ fields: grounded })} canAccept={false} />
  ),
};

// Both omission reasons at once, because the two are what a reader has to tell
// apart: silent in the file, versus stated too vaguely to stage.
export const OmittedForBothReasons: Story = {
  render: () => (
    <Panel
      reading={extraction({
        fields: [grounded[0], grounded[1]],
        omitted: [
          { field: "expected_close_date", reason: "not_stated_in_file" },
          { field: "amount_minor", reason: "not_confidently_stated" },
        ],
      })}
    />
  ),
};

// A finished reading that grounded nothing is a correct answer, and it has to
// say why or it reads as a broken feature.
export const GroundedNothing: Story = {
  render: () => (
    <Panel
      reading={extraction({
        status_detail:
          "This file is a signed cover letter; it states no terms.",
      })}
    />
  ),
};

export const Failed: Story = {
  render: () => (
    <Panel
      reading={extraction({
        status: "failed",
        status_detail: "The file is a scan with no text layer.",
      })}
    />
  ),
};
