// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ImportCard } from "./import";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Bringing a customer's file in. On the settings page the import is one row —
// an import is an ACT, not an answer this installation holds — and the wizard
// that performs it is the page behind the row's verb, at #/settings/import/run.
//
// What is catalogued here is the row, the two answers the card gives before
// anybody chooses a file, the run an earlier visit left parked, and the question
// asked before a profiled file is thrown away.
function story(allow: Parameters<typeof meRoute>[0], subpage?: string) {
  return () => {
    // A story states its own preconditions, including the absence of one.
    // `PickedUpFromEarlier` plants a run id in storage, and storage outlives a
    // story: the capture harness drives every story through ONE page, so the
    // next one inherits it and shows a run this story never mentioned.
    globalThis.localStorage.removeItem("margince.import.run");
    installFetchStub({ "GET /me": meRoute(allow) });
    return (
      <StoryProviders>
        <ImportCard subpage={subpage} />
      </StoryProviders>
    );
  };
}

const RUN = "run";

const OPERATOR = { import_run: ["create", "read", "update"] } as const;

const meta: Meta<typeof ImportCard> = {
  title: "Settings/Data/Data import/Import",
  component: ImportCard,
};
export default meta;
type Story = StoryObj<typeof ImportCard>;

// The card at rest: one row, naming the act on the left and carrying the verb on
// the right, at the same x as every other row on the page. Nothing about a file
// is on screen yet, because nothing about a file is a setting.
export const TheRow: Story = { render: story(OPERATOR) };

// Maintenance opens on the admin role OR an embedding-reindex read, so a seat
// holding only the latter reaches this page. It is told the import exists and is
// not theirs to run — an absent card would say the installation cannot import.
export const Withheld: Story = {
  render: story({ embedding_reindex: ["read"] }),
};

// The wizard's first step: what the rows are, and the file to read them from.
export const ChoosingAFile: Story = { render: story(OPERATOR, RUN) };

// The first step in dark, and the reason it is dark rather than narrow: this
// card's own sheet opens by declaring that every quiet line on it reads --textMeta
// and not --textMuted, because --textMuted measures 1.54:1 here while --textMeta
// is the canonical AA small-text role — a rule written against the LIGHT palette
// and, until this story, never looked at once both tokens re-resolved. The lines
// under test are the object hint and the file-format sentence, sitting beside a
// SegmentedControl whose selected segment is the loudest thing on the page.
//
// A narrow variant would prove less: the flow past the first step needs a real
// file drop, so the wide mapping table and its TableScroll box — the parts
// that have a width problem to have — are not reachable from a story at all.
export const ChoosingAFileDark: Story = {
  globals: { theme: "dark" },
  render: story(OPERATOR, RUN),
};

// The one state past the first step a story CAN reach, and the reason it can is
// the point of the state: it needs no file, only the run id an earlier visit
// left in storage and the two reads that answer for a run by id. This is what a
// reader sees coming back from the Leads list after editing the one row they do
// not want reversed — the outcome, the notice saying where it came from, and the
// undo that used to vanish the moment they navigated away.
//
// The reference it plants OUTLIVES it — storage is not per-story — so the shared
// helper above clears it rather than every other story trusting this one to
// leave the page as it found it.
function parked(subpage?: string) {
  return () => {
    globalThis.localStorage.setItem("margince.import.run", "019ff-run");
    installFetchStub({
      "GET /me": meRoute(OPERATOR),
      "GET /imports/019ff-run/report": () =>
        jsonResponse({
          run_id: "019ff-run",
          status: "complete",
          rows_read: 4,
          disposition: { created: 3, updated: 0, unchanged: 0, skipped: 1 },
          issues: [],
          source_key_used: "Email",
        }),
      "GET /imports/019ff-run": () =>
        jsonResponse({
          id: "019ff-run",
          connector: "csv",
          object: "lead",
          status: "complete",
          checkpoint: 4,
          source: "import_api",
          created_at: "2026-08-17T14:12:00Z",
          updated_at: "2026-08-17T14:12:40Z",
        }),
    });
    return (
      <StoryProviders>
        <ImportCard subpage={subpage} />
      </StoryProviders>
    );
  };
}

export const PickedUpFromEarlier: Story = { render: parked(RUN) };

// The row while that run is parked: an operator who does not know their last
// import stopped half-way cannot finish it, so the verb says there is one.
export const ParkedRunRow: Story = { render: parked() };

// Another row type starts the flow over, so a profiled file is asked about
// before it is dropped rather than lost to a misclick.
export const AskingBeforeStartingOver: Story = {
  render: () => {
    globalThis.localStorage.removeItem("margince.import.run");
    installFetchStub({
      "GET /me": meRoute(OPERATOR),
      "POST /imports/sources": () =>
        jsonResponse({
          source_ref: "ws/import/abc",
          object: "lead",
          rows_profiled: 2,
          columns: [{ header: "Email", fill_rate: 1, samples: ["ada@x.test"] }],
          suggested_mapping: { Email: "email" },
          targets: ["full_name", "email"],
        }),
    });
    return (
      <StoryProviders>
        <ImportCard subpage={RUN} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.upload(
      await canvas.findByLabelText("CSV file"),
      new File(["Email\nada@x.test\n"], "estate.csv", { type: "text/csv" }),
    );
    await canvas.findByRole("row", { name: /Email/ });
    await userEvent.click(canvas.getByRole("button", { name: "Companies" }));
    const page = within(canvasElement.ownerDocument.body);
    await expect(await page.findByRole("dialog")).toBeVisible();
  },
};
