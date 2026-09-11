// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TagResultScreen } from "./tagresult";

type TagUsage = components["schemas"]["TagUsage"];

// The page behind a tag pill, whose whole job is naming WHICH records carry
// the word. The three groups are three separate reads gated on the counts the
// tag read returned, so what a story sets those counts to is what decides the
// shape of the page — which is why they are the axis the stories vary.
//
// The screen heads itself and reaches for no capability, so there is no session
// probe here and no shell title above it; `fullscreen` because the page draws
// its own `.wrap`.
const meta: Meta = {
  title: "Records/Tag page",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

const TAG = "01a0641b-db0c-7b92-8564-6c843bf58df3";

function tagRead(usage: TagUsage) {
  return () =>
    jsonResponse({
      id: TAG,
      name: "Automation World 2026",
      color: "slate",
      description:
        "Everyone we met at the hall, and the deals that came of it.",
      version: 1,
      usage,
    });
}

const contacts = [
  { id: "p-1", full_name: "Katrin Hofmann" },
  { id: "p-2", full_name: "Devrim Aksoy" },
];
const companies = [
  { id: "o-1", display_name: "MiTek" },
  { id: "o-2", display_name: "Nordfracht" },
];
const deals = [{ id: "d-1", name: "netcare GmbH — Einführung" }];

// All three record types carry the word: one group per type, each naming its
// own rows, and none of them offering "View all" — every group here is shorter
// than the preview, so there is nothing further to show.
export const RecordsInEveryGroup: Story = {
  render: () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ contacts: 2, companies: 2, deals: 1 }),
      "GET /contacts": () => jsonResponse({ data: contacts }),
      "GET /companies": () => jsonResponse({ data: companies }),
      "GET /deals": () => jsonResponse({ data: deals }),
    });
    return (
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>
    );
  },
};

// A word nothing carries: one panel saying so, rather than three cards each
// reporting a zero. The counts come from the tag read, so no group read runs.
export const NothingCarriesIt: Story = {
  render: () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ contacts: 0, companies: 0, deals: 0 }),
    });
    return (
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>
    );
  },
};

// The rows are still on the wire. The counts already arrived, so each group
// has its heading and a skeleton sized to what it is about to show — the state
// a reader on a slow connection actually sees, since the page itself renders
// nothing at all until the tag read settles.
export const RowsStillLoading: Story = {
  render: () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ contacts: 2, companies: 2, deals: 1 }),
      "GET /contacts": () => new Promise<Response>(() => {}),
      "GET /companies": () => new Promise<Response>(() => {}),
      "GET /deals": () => new Promise<Response>(() => {}),
    });
    return (
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>
    );
  },
};
