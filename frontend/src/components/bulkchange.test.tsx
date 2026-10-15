// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
  BulkUndoProvider,
} from "./bulkchange";

type BulkChangePreview = components["schemas"]["BulkChangePreview"];
type BulkChangeResult = components["schemas"]["BulkChangeResult"];

// The bulk dialog asks the server what a change would do before it does it,
// and runs exactly the selection it previewed. These tests hold the three
// promises that makes: the reader sees what will change and what will not, the
// confirm carries the token and each row's version, and a preview that would
// change nothing offers nothing to confirm.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Sent = { path: string; body: unknown; idempotencyKey: string | null };

/** Answers the bulk calls, their undo and the user roster; records every POST. */
type UndoStub = Readonly<{
  preview: BulkChangePreview;
  result: BulkChangeResult;
  failures?: number;
}>;

function stubBulk(
  preview: BulkChangePreview,
  result?: BulkChangeResult,
  executeFailures = 0,
  undo?: UndoStub,
) {
  const sent: Sent[] = [];
  let failuresLeft = executeFailures;
  let undoFailuresLeft = undo?.failures ?? 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url, "https://test.local").pathname;
      if (input.method === "POST") {
        // The undo's preview carries no body at all.
        const text = await input.clone().text();
        sent.push({
          path,
          body: text === "" ? undefined : JSON.parse(text),
          idempotencyKey: input.headers.get("Idempotency-Key"),
        });
      }
      if (path === "/v1/bulk/preview") {
        return json(preview);
      }
      if (path.endsWith("/undo/preview")) {
        return json(undo?.preview);
      }
      if (path.endsWith("/undo")) {
        if (undoFailuresLeft > 0) {
          undoFailuresLeft -= 1;
          throw new TypeError("network connection lost");
        }
        return json(undo?.result);
      }
      if (path === "/v1/bulk/execute") {
        if (failuresLeft > 0) {
          failuresLeft -= 1;
          throw new TypeError("network connection lost");
        }
        return json(result);
      }
      return json({
        data: [
          { id: "u-mila", display_name: "Mila Brandt" },
          { id: "u-jonas", display_name: "Jonas Weber" },
        ],
        page: { has_more: false },
      });
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <BulkUndoProvider>{ui}</BulkUndoProvider>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const REASSIGN: BulkChangeRequest = {
  recordType: "contact",
  verb: "reassign_owner",
  ownerId: "u-jonas",
  openId: "open-1",
  rows: [
    { id: "c-1", version: 3, label: "Anna Weber" },
    { id: "c-2", version: 7, label: "Ben Ott" },
    { id: "c-3", version: 2, label: "Clara Ruiz" },
  ],
};

const REASSIGN_PREVIEW: BulkChangePreview = {
  record_type: "contact",
  verb: "reassign_owner",
  count: 2,
  affected: ["c-1", "c-2"],
  excluded: [{ id: "c-3", reason: "no_change" }],
  sample: [
    {
      id: "c-1",
      label: "Anna Weber",
      before: { owner_id: "u-mila", archived: false },
      after: { owner_id: "u-jonas", archived: false },
    },
  ],
  requires_confirmation: false,
};

const UNDO_PREVIEW: BulkChangePreview = {
  record_type: "contact",
  verb: "reassign_owner",
  count: 1,
  affected: ["c-1"],
  excluded: [{ id: "c-2", reason: "changed_since_batch" }],
  sample: [
    {
      id: "c-1",
      label: "Anna Weber",
      before: { owner_id: "u-jonas", archived: false },
      after: { owner_id: "u-mila", archived: false },
    },
  ],
  requires_confirmation: false,
  confirm_token: "tok-undo",
};

const UNDONE: BulkChangeResult = {
  batch_id: "b-2",
  undo_of: "b-1",
  changed: 1,
  skipped: [{ id: "c-2", reason: "changed_since_batch" }],
};

/** Hosts the dialog as a list's bulk bar does: it closes once the change ran. */
function ClosingHost({ request }: Readonly<{ request: BulkChangeRequest }>) {
  const [open, setOpen] = useState<BulkChangeRequest | null>(request);
  return (
    <BulkChangeDialog
      request={open}
      onClose={() => setOpen(null)}
      onDone={() => setOpen(null)}
    />
  );
}

describe("the bulk change preview", () => {
  it("says how many records change, names each one left alone and why, and samples the owners", async () => {
    stubBulk(REASSIGN_PREVIEW);
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(
        "This changes 2 of the 3 selected records.",
      ),
    ).toBeInTheDocument();
    const excluded = within(dialog)
      .getByText("Clara Ruiz")
      .closest("tr") as HTMLElement;
    expect(
      within(excluded).getByText(en["bulk.reason.no_change"]),
    ).toBeInTheDocument();
    const sample = within(dialog)
      .getByText("Anna Weber")
      .closest("tr") as HTMLElement;
    expect(await within(sample).findByText("Mila Brandt")).toBeInTheDocument();
    expect(within(sample).getByText("Jonas Weber")).toBeInTheDocument();
  });

  it("names each refusal by its rule's code, falling back to the server's words", async () => {
    stubBulk({
      ...REASSIGN_PREVIEW,
      count: 0,
      affected: [],
      sample: [],
      excluded: [
        { id: "c-1", reason: "refused", code: "locked", message: "on hold" },
        {
          id: "c-2",
          reason: "refused",
          code: "a_rule_this_client_does_not_know",
          message: "Held by a rule not listed here.",
        },
        { id: "c-3", reason: "not_previewed" },
      ],
    });
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    const reasonOf = async (name: string) =>
      within(
        (await screen.findByText(name)).closest("tr") as HTMLElement,
      ).getAllByRole("cell")[1].textContent;
    expect(await reasonOf("Anna Weber")).toBe(en["bulk.refusal.locked"]);
    expect(await reasonOf("Ben Ott")).toBe("Held by a rule not listed here.");
    expect(await reasonOf("Clara Ruiz")).toBe(en["bulk.reason.not_previewed"]);
  });

  it("draws an archive sample as active going to archived", async () => {
    stubBulk({
      ...REASSIGN_PREVIEW,
      verb: "archive",
      excluded: [],
      sample: [
        {
          id: "c-1",
          label: "Anna Weber",
          before: { owner_id: "u-mila", archived: false },
          after: { owner_id: "u-mila", archived: true },
        },
      ],
    });
    render(
      <BulkChangeDialog
        request={{ ...REASSIGN, verb: "archive", ownerId: undefined }}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    const row = (await screen.findByText("Anna Weber")).closest(
      "tr",
    ) as HTMLElement;
    expect(within(row).getByText(en["bulk.stateActive"])).toBeInTheDocument();
    expect(within(row).getByText(en["record.archived"])).toBeInTheDocument();
    // The owner is not what an archive moves, so the sample does not show it.
    expect(within(row).queryByText("Mila Brandt")).toBeNull();
  });

  it("offers only Close when nothing would change", async () => {
    stubBulk({
      ...REASSIGN_PREVIEW,
      count: 0,
      affected: [],
      sample: [],
      excluded: REASSIGN.rows.map((row) => ({
        id: row.id,
        reason: "no_change" as const,
      })),
    });
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(en["bulk.nothing"]),
    ).toBeInTheDocument();
    expect(
      within(dialog).queryByRole("button", {
        name: en["bulk.confirmReassign"],
      }),
    ).toBeNull();
    expect(
      within(dialog).queryByRole("button", { name: en["create.cancel"] }),
    ).toBeNull();
    // The footer's Close, beside the dialog's own corner control of that name.
    expect(
      within(dialog).getAllByRole("button", { name: en["common.close"] })
        .length,
    ).toBeGreaterThan(0);
  });

  it("warns before a change that needs confirmation", async () => {
    stubBulk({
      ...REASSIGN_PREVIEW,
      requires_confirmation: true,
      confirm_token: "tok-1",
    });
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    expect(await screen.findByText(en["bulk.largeTitle"])).toBeInTheDocument();
  });
});

describe("confirming a bulk change", () => {
  it("runs the previewed selection once, with the token, each row's version and an idempotency key", async () => {
    const sent = stubBulk(
      {
        ...REASSIGN_PREVIEW,
        requires_confirmation: true,
        confirm_token: "tok-1",
      },
      {
        batch_id: "b-1",
        changed: 2,
        skipped: [{ id: "c-3", reason: "no_change" }],
      },
    );
    const onDone = vi.fn();
    const user = userEvent.setup();
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={onDone}
      />,
    );

    await user.click(
      await screen.findByRole("button", { name: en["bulk.confirmReassign"] }),
    );

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    expect(onDone).toHaveBeenCalledWith(
      expect.objectContaining({ batch_id: "b-1", changed: 2 }),
    );
    const items = [
      { id: "c-1", version: 3 },
      { id: "c-2", version: 7 },
      { id: "c-3", version: 2 },
    ];
    expect(sent.map((request) => request.path)).toEqual([
      "/v1/bulk/preview",
      "/v1/bulk/execute",
    ]);
    expect(sent[0].body).toEqual({
      record_type: "contact",
      verb: "reassign_owner",
      items,
      owner_id: "u-jonas",
    });
    expect(sent[1].body).toEqual({
      record_type: "contact",
      verb: "reassign_owner",
      items,
      owner_id: "u-jonas",
      confirm_token: "tok-1",
    });
    expect(sent[1].idempotencyKey).toBeTruthy();
    expect(
      await screen.findByText("2 contacts changed. 1 was left unchanged."),
    ).toBeInTheDocument();
  });

  it("offers Undo, which previews the undo and runs it only once the reader confirms", async () => {
    const sent = stubBulk(
      REASSIGN_PREVIEW,
      {
        batch_id: "b-1",
        changed: 2,
        skipped: [{ id: "c-3", reason: "no_change" }],
      },
      0,
      { preview: UNDO_PREVIEW, result: UNDONE },
    );
    const user = userEvent.setup();
    render(<ClosingHost request={REASSIGN} />);

    await user.click(
      await screen.findByRole("button", { name: en["bulk.confirmReassign"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        en["bulk.titleUndo"].replace("{unit}", en["unit.contacts"]),
      ),
    ).toBeInTheDocument();
    expect(
      await within(dialog).findByText(en["bulk.reason.changed_since_batch"]),
    ).toBeInTheDocument();
    expect(sent.map((request) => request.path)).toEqual([
      "/v1/bulk/preview",
      "/v1/bulk/execute",
      "/v1/bulk/b-1/undo/preview",
    ]);

    await user.click(
      within(dialog).getByRole("button", { name: en["bulk.confirmUndo"] }),
    );

    expect(
      await screen.findByText("1 contact put back. 1 was left unchanged."),
    ).toBeInTheDocument();
    expect(sent.at(-1)?.path).toBe("/v1/bulk/b-1/undo");
    expect(sent.at(-1)?.body).toEqual({ confirm_token: "tok-undo" });
    expect(sent.at(-1)?.idempotencyKey).toBeTruthy();
    // An undo is not undone in turn, so its own toast offers nothing.
    expect(
      screen.queryByRole("button", { name: en["common.undo"] }),
    ).toBeNull();
  });

  it("retries a lost undo answer with the same token and key, so the undo runs once", async () => {
    const sent = stubBulk(
      REASSIGN_PREVIEW,
      { batch_id: "b-1", changed: 2, skipped: [] },
      0,
      { preview: UNDO_PREVIEW, result: UNDONE, failures: 1 },
    );
    const user = userEvent.setup();
    render(<ClosingHost request={REASSIGN} />);

    await user.click(
      await screen.findByRole("button", { name: en["bulk.confirmReassign"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );
    const confirm = await screen.findByRole("button", {
      name: en["bulk.confirmUndo"],
    });
    await user.click(confirm);
    await waitFor(() => expect(confirm).not.toBeDisabled());
    await user.click(confirm);

    expect(
      await screen.findByText("1 contact put back. 1 was left unchanged."),
    ).toBeInTheDocument();
    const undos = sent.filter(
      (request) => request.path === "/v1/bulk/b-1/undo",
    );
    expect(undos).toHaveLength(2);
    expect(undos[1].idempotencyKey).toBe(undos[0].idempotencyKey);
    expect(undos[1].body).toEqual(undos[0].body);
  });

  it("retries a lost answer under the same idempotency key, so the change runs once", async () => {
    const sent = stubBulk(
      REASSIGN_PREVIEW,
      { batch_id: "b-1", changed: 2, skipped: [] },
      1,
    );
    const onDone = vi.fn();
    const user = userEvent.setup();
    render(
      <BulkChangeDialog
        request={REASSIGN}
        onClose={() => {}}
        onDone={onDone}
      />,
    );

    const confirm = await screen.findByRole("button", {
      name: en["bulk.confirmReassign"],
    });
    await user.click(confirm);
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    await user.click(confirm);

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    const executes = sent.filter((call) => call.path === "/v1/bulk/execute");
    expect(executes).toHaveLength(2);
    expect(executes[0].idempotencyKey).toBeTruthy();
    expect(executes[1].idempotencyKey).toBe(executes[0].idempotencyKey);
  });

  it("sends no owner with an archive", async () => {
    const sent = stubBulk(
      { ...REASSIGN_PREVIEW, verb: "archive" },
      { batch_id: "b-2", changed: 2, skipped: [] },
    );
    const user = userEvent.setup();
    render(
      <BulkChangeDialog
        request={{ ...REASSIGN, verb: "archive", ownerId: "u-jonas" }}
        onClose={() => {}}
        onDone={() => {}}
      />,
    );

    await user.click(
      await screen.findByRole("button", {
        name: en["bulk.confirmArchive"].replace("{unit}", en["unit.contacts"]),
      }),
    );

    await waitFor(() => expect(sent).toHaveLength(2));
    for (const request of sent) {
      expect(request.body).not.toHaveProperty("owner_id");
    }
  });
});
