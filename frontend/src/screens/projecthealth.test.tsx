/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProjectHealth } from "./projecthealth";
import { ProjectHealthModal } from "./projecthealthmodal";

// The panel described how a delivery is going and offered no way to say so.
// What these assert is that the way in appears only where a write would be
// accepted, that correcting is offered on the reading that STANDS and on no
// other, and that the note rule the server enforces is stated in the form
// rather than discovered through a refusal.

const PROJECT_ID = "11111111-1111-1111-1111-111111111111";

const CURRENT = {
  id: "h2",
  project_id: PROJECT_ID,
  state: "at_risk",
  note: "Feed is late.",
  assessed_at: "2026-09-13T09:00:00Z",
  created_at: "2026-09-13T09:00:00Z",
  superseded: false,
  source: "human",
};

const SUPERSEDED = {
  ...CURRENT,
  id: "h1",
  note: "Feed is very late.",
  superseded: true,
};

function stubFetch(rows: unknown[], opts: Readonly<{ fail?: boolean }> = {}) {
  vi.stubGlobal(
    "fetch",
    // Every route this panel reads answers the same way, so the stub does not
    // branch on the request: the assessments list IS the only read.
    vi.fn(async () => {
      if (opts.fail) {
        return new Response(JSON.stringify({ title: "nope" }), {
          status: 500,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ data: rows }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return rtlRender(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("invites a reading on a project nobody has judged", async () => {
  stubFetch([]);
  render(<ProjectHealth projectId={PROJECT_ID} onRecord={() => {}} />);
  expect(await screen.findByText(/Nobody has judged this yet/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Record a reading" })).toBeTruthy();
});

it("offers no way in on a project this reader cannot write", async () => {
  stubFetch([CURRENT]);
  render(<ProjectHealth projectId={PROJECT_ID} readOnly onRecord={() => {}} />);
  // The judgement still renders: how a delivery is going is a fact a read-only
  // reader is entitled to.
  expect((await screen.findAllByText(/Feed is late/)).length).toBeGreaterThan(
    0,
  );
  expect(screen.queryByRole("button", { name: "Record a reading" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Correct this" })).toBeNull();
});

it("offers no way in over a read that failed", async () => {
  stubFetch([], { fail: true });
  render(<ProjectHealth projectId={PROJECT_ID} onRecord={() => {}} />);
  expect(await screen.findByText(/did not load/i)).toBeTruthy();
  // An unjudged project and an unloaded one look identical. Inviting a reading
  // over the second invites a second judgement beside one that may stand.
  expect(screen.queryByRole("button", { name: "Record a reading" })).toBeNull();
});

it("offers a correction on the reading that stands and on no other", async () => {
  const corrected: string[] = [];
  stubFetch([CURRENT, SUPERSEDED]);
  render(
    <ProjectHealth
      projectId={PROJECT_ID}
      onRecord={() => {}}
      onCorrect={(row) => corrected.push(row.id)}
    />,
  );
  const buttons = await screen.findAllByRole("button", {
    name: "Correct this",
  });
  // One, not two: the superseded row has already been answered and the server
  // refuses a second correction of it.
  expect(buttons).toHaveLength(1);
  await userEvent.click(buttons[0]);
  expect(corrected).toEqual(["h2"]);
});

it("will not send a troubled reading without saying what is wrong", async () => {
  stubFetch([]);
  render(<ProjectHealthModal open onClose={() => {}} projectId={PROJECT_ID} />);
  // On track needs no note: a delivery going fine has nothing to explain.
  expect(
    screen.getByRole("button", { name: "Record it" }).hasAttribute("disabled"),
  ).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "At risk" }));
  // The server refuses this too. Stating it here is what stops a reader
  // learning the rule from a 422 after writing their judgement.
  expect(
    screen.getByRole("button", { name: "Record it" }).hasAttribute("disabled"),
  ).toBe(true);
  await userEvent.type(screen.getByRole("textbox"), "Partner is behind.");
  expect(
    screen.getByRole("button", { name: "Record it" }).hasAttribute("disabled"),
  ).toBe(false);
});

it("starts a correction from what the reading actually said", async () => {
  stubFetch([]);
  render(
    <ProjectHealthModal
      open
      onClose={() => {}}
      projectId={PROJECT_ID}
      correcting={{ id: "h2", state: "at_risk", note: "Feed is late." }}
    />,
  );
  // A reader fixing one word should not have to restate the judgement.
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe(
    "Feed is late.",
  );
  expect(screen.getByText(/keeps its original date/)).toBeTruthy();
});
