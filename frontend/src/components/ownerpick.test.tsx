// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../lib/problem";
import { meFixture } from "../testing/mefixture";
import { type OpenedRecord, OwnerPick } from "./ownerpick";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const COLLEAGUES = [
  { id: "u-42", display_name: "Jane Doe", email: "jane@example.test" },
  { id: "u-7", display_name: "Omar Haddad", email: "omar@example.test" },
  {
    id: "u-read",
    display_name: "Rita Reader",
    email: "rita@example.test",
    can_own_records: false,
  },
  {
    id: "a-1",
    display_name: "Follow-up agent",
    email: "agent@example.test",
    is_agent: true,
  },
];

type RosterAnswer = "everyone" | "refused" | "endless";

function stubRoster(
  answer: RosterAnswer = "everyone",
  viewer = "u-viewer",
  colleagues: readonly unknown[] = COLLEAGUES,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (new URL(url, "https://test.local").pathname.endsWith("/me")) {
        const me = meFixture();
        return jsonResponse({ ...me, user: { ...me.user, id: viewer } });
      }
      if (!url.includes("/users")) {
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }
      if (answer === "refused") {
        return jsonResponse(
          { title: "Forbidden", status: 403, detail: "the roster is closed" },
          403,
        );
      }
      const laterPage = new URL(url, "https://test.local").searchParams.has(
        "cursor",
      );
      return jsonResponse({
        data: laterPage ? [] : colleagues,
        page: { next_cursor: answer === "endless" ? "more" : null },
      });
    }),
  );
}

type Harness = Readonly<{
  ownerId?: string | null;
  version?: number;
  assign?: (ownerId: string, opened: OpenedRecord) => Promise<unknown>;
  unassign?: (opened: OpenedRecord) => Promise<unknown>;
  onAssigned?: () => void;
  pending?: boolean;
}>;

function render(props: Harness = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui = ({
    ownerId = null,
    version = 5,
    assign = vi.fn(async () => undefined),
    unassign,
    onAssigned,
    pending,
  }: Harness) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <OwnerPick
            record={{ id: "r-1", version, owner_id: ownerId }}
            assign={assign}
            unassign={unassign}
            invalidate="projects"
            recordKey="project"
            onAssigned={onAssigned}
            pending={pending}
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = rtlRender(ui(props));
  return { client, rerender: (next: Harness) => rerender(ui(next)) };
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Change owner" }));
  return within(await screen.findByRole("dialog"));
}

// A write the test answers when it chooses.
function heldWrite() {
  let settle: (refusal?: ProblemError) => void = () => {};
  const assign = vi.fn(
    () =>
      new Promise<void>((resolve, reject) => {
        settle = (refusal) => (refusal ? reject(refusal) : resolve());
      }),
  );
  return { assign, settle: (refusal?: ProblemError) => settle(refusal) };
}

describe("OwnerPick", () => {
  it("offers only colleagues who may own records, and a pick hands the record over once", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const onAssigned = vi.fn();
    const user = userEvent.setup();
    render({ assign, onAssigned });

    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Jane Doe" });
    expect(
      panel.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["Jane Doe", "Omar Haddad"]);
    await user.click(panel.getByRole("option", { name: "Jane Doe" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(assign).toHaveBeenCalledTimes(1);
    expect(assign).toHaveBeenCalledWith(
      "u-42",
      expect.objectContaining({ id: "r-1", version: 5 }),
    );
    expect(onAssigned).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Assigned to Jane Doe")).toBeInTheDocument();
  });

  // The roster answers in no useful order; the list reads by name, viewer first.
  // Mutation: drop the sort or the viewer's move and the order breaks.
  it("lists the viewer first, then colleagues by name", async () => {
    stubRoster("everyone", "u-7", [
      { id: "u-9", display_name: "Zoe Weber", email: "zoe@example.test" },
      ...COLLEAGUES,
      { id: "u-3", display_name: "Anna Berg", email: "anna@example.test" },
    ]);
    const user = userEvent.setup();
    render();

    const panel = await openPicker(user);
    await waitFor(() =>
      expect(
        panel.getAllByRole("option").map((option) => option.textContent),
      ).toEqual(["Omar Haddad", "Anna Berg", "Jane Doe", "Zoe Weber"]),
    );
  });

  it("offers Unassigned first where the record may go back to the queue, and a pick clears the owner", async () => {
    stubRoster("everyone", "u-7");
    const assign = vi.fn(async () => undefined);
    const unassign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    render({ ownerId: "u-7", assign, unassign });

    const panel = await openPicker(user);
    await waitFor(() =>
      expect(
        panel.getAllByRole("option").map((option) => option.textContent),
      ).toEqual(["Unassigned", "Omar Haddad", "Jane Doe"]),
    );
    await user.click(panel.getByRole("option", { name: "Unassigned" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(unassign).toHaveBeenCalledWith(
      expect.objectContaining({ id: "r-1", version: 5 }),
    );
    expect(assign).not.toHaveBeenCalled();
    expect(
      await screen.findByText("Returned to the unassigned queue"),
    ).toBeInTheDocument();
  });

  it("marks Unassigned on a record nobody owns, and picking it writes nothing", async () => {
    stubRoster();
    const unassign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    render({ ownerId: null, unassign });

    const panel = await openPicker(user);
    const nobody = await panel.findByRole("option", { name: "Unassigned" });
    expect(nobody).toHaveAttribute("aria-selected", "true");
    await user.click(nobody);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(unassign).not.toHaveBeenCalled();
  });

  it("names the picked colleague in the toast when the roster is gone by then", async () => {
    stubRoster();
    const write = heldWrite();
    const user = userEvent.setup();
    const { client } = render({ assign: write.assign });

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));
    act(() =>
      client
        .getQueryCache()
        .find({ queryKey: ["users"] })
        ?.reset(),
    );
    await waitFor(() => expect(panel.queryAllByRole("option")).toHaveLength(0));
    write.settle();

    expect(await screen.findByText("Assigned to Jane Doe")).toBeInTheDocument();
  });

  it("marks the current owner, and picking them again writes nothing", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const onAssigned = vi.fn();
    const user = userEvent.setup();
    render({ ownerId: "u-7", assign, onAssigned });

    const panel = await openPicker(user);
    const current = await panel.findByRole("option", { name: "Omar Haddad" });
    expect(current).toHaveAttribute("aria-selected", "true");
    await user.click(current);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(assign).not.toHaveBeenCalled();
    expect(onAssigned).not.toHaveBeenCalled();
  });

  it("finds a colleague by email as well as by name", async () => {
    stubRoster();
    const user = userEvent.setup();
    render();

    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Jane Doe" });
    await user.type(panel.getByRole("combobox"), "omar@");

    expect(
      panel.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["Omar Haddad"]);
  });

  it("writes once for a double press on a colleague", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    render({ assign });

    const panel = await openPicker(user);
    const option = await panel.findByRole("option", { name: "Jane Doe" });
    // One act holds both presses before any render, so `pending` is still
    // false for the second and only the list's latch can refuse it.
    act(() => {
      fireEvent.click(option);
      fireEvent.click(option);
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(assign).toHaveBeenCalledTimes(1);
  });

  it("stays open through Escape while the write is out, so a late refusal is read", async () => {
    stubRoster();
    const write = heldWrite();
    const user = userEvent.setup();
    render({ assign: write.assign });

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));
    await waitFor(() => expect(write.assign).toHaveBeenCalledTimes(1));

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    write.settle(new ProblemError({ detail: "the record was changed" }));
    expect(await panel.findByRole("alert")).toHaveTextContent(
      "the record was changed",
    );
  });

  it("keeps the list open over a refusal, saying why, and reopens clean", async () => {
    stubRoster();
    const onAssigned = vi.fn();
    const user = userEvent.setup();
    render({
      assign: async () => {
        throw new ProblemError({ detail: "the record was changed" });
      },
      onAssigned,
    });

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));

    expect(await panel.findByRole("alert")).toHaveTextContent(
      "the record was changed",
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(onAssigned).not.toHaveBeenCalled();

    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Change owner" })).toBe(
      document.activeElement,
    );
    const reopened = await openPicker(user);
    await reopened.findByRole("option", { name: "Jane Doe" });
    expect(reopened.queryByRole("alert")).toBeNull();
  });

  it("says why when the colleague list could not be read, instead of offering nobody", async () => {
    stubRoster("refused");
    const user = userEvent.setup();
    render();

    const panel = await openPicker(user);

    expect(await panel.findByText("the roster is closed")).toBeInTheDocument();
    expect(panel.queryByText("No match")).toBeNull();
    expect(panel.queryAllByRole("option")).toHaveLength(0);
  });

  it("says the list is part of the workspace when the roster walk stopped short", async () => {
    stubRoster("endless");
    const user = userEvent.setup();
    render();

    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Jane Doe" });

    expect(panel.getByText("Showing part of the list")).toBeInTheDocument();
  });

  it("writes against the record as the list opened on it, though it changed since", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    const { rerender } = render({ assign, version: 5 });

    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Jane Doe" });
    rerender({ assign, version: 6 });
    await user.click(panel.getByRole("option", { name: "Jane Doe" }));

    await waitFor(() => expect(assign).toHaveBeenCalledTimes(1));
    expect(assign).toHaveBeenCalledWith(
      "u-42",
      expect.objectContaining({ version: 5 }),
    );
  });

  it("refuses a pick while another write on the record is out", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    render({ assign, pending: true });

    const panel = await openPicker(user);
    const option = await panel.findByRole("option", { name: "Jane Doe" });
    expect(option).toHaveAttribute("aria-disabled", "true");
    await user.click(option);

    expect(assign).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("lists the viewer first, under their own name", async () => {
    stubRoster("everyone", "u-7");
    const user = userEvent.setup();
    render();

    const panel = await openPicker(user);
    await waitFor(() =>
      expect(
        panel.getAllByRole("option").map((option) => option.textContent),
      ).toEqual(["Omar Haddad", "Jane Doe"]),
    );
  });

  it("opens from a caller's own control, writes against the record as it opened, and hands focus back", async () => {
    stubRoster();
    const assign = vi.fn(async () => undefined);
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    function Section({ version }: Readonly<{ version: number }>) {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Edit Ownership
          </button>
          <OwnerPick
            opener="caller"
            open={open}
            onClose={() => setOpen(false)}
            record={{ id: "r-1", version, owner_id: null }}
            assign={assign}
            invalidate="projects"
            recordKey="project"
          />
        </>
      );
    }
    const ui = (version: number) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <Section version={version} />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = rtlRender(ui(5));
    expect(screen.getAllByRole("button")).toHaveLength(1);

    const edit = screen.getByRole("button", { name: "Edit Ownership" });
    await user.click(edit);
    const sheet = within(
      await screen.findByRole("dialog", { name: "Change owner" }),
    );
    await sheet.findByRole("option", { name: "Jane Doe" });
    rerender(ui(6));
    await user.click(sheet.getByRole("option", { name: "Jane Doe" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(assign).toHaveBeenCalledWith(
      "u-42",
      expect.objectContaining({ version: 5 }),
    );
    expect(document.activeElement).toBe(edit);
  });
});
