/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CallAction } from "../design-system/callaction";
import { telUri } from "../format/contacturi";
import { type CallStart, CallStartProvider } from "../lib/callstart";
import { useMe } from "../lib/session";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { CallLogHost, CallLogInline, useCallLogFor } from "./calllog.host";

afterEach(cleanup);

const DIALS = "+49 30 1234 5678";
const DANA = { entityType: "contact", entityId: "p-1" } as const;

// No press before the seat answers: a refusal must not pass on a /me that
// never arrived.
function SessionAnswered() {
  return useMe().data ? <p>session answered</p> : null;
}

async function mountHost(seat: "full" | "read" = "full") {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"], contact: ["read"] }, { seat }),
    "GET /contacts/p-1": () =>
      jsonResponse({ id: "p-1", full_name: "Dana Buyer" }),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse(
        { id: "a-9", kind: "call", occurred_at: "2026-09-08T09:00:00Z" },
        201,
      );
    },
  });
  render(
    <StoryProviders>
      <CallLogHost>
        <SessionAnswered />
        <CallAction appearance="value" number={DIALS} record={DANA} />
      </CallLogHost>
    </StoryProviders>,
  );
  await screen.findByText("session answered");
  return { posted, link: screen.getByRole("link", { name: DIALS }) };
}

describe("a call started under the shell's host", () => {
  it("still dials, and opens Log activity on Call", async () => {
    const { link } = await mountHost();

    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    expect(fireEvent.click(link)).toBe(true);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    expect(
      within(dialog).getByRole("combobox", { name: "Type" }).textContent,
    ).toContain("Call");
    expect(await within(dialog).findByText("Dana Buyer")).toBeTruthy();
  });

  it("keeps the open log and its words when the page under it changes", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
    });
    const page = (shown: ReactNode) => (
      <StoryProviders>
        <CallLogHost>
          <SessionAnswered />
          {shown}
        </CallLogHost>
      </StoryProviders>
    );
    const { rerender } = render(
      page(<CallAction appearance="value" number={DIALS} record={DANA} />),
    );
    await screen.findByText("session answered");
    fireEvent.click(screen.getByRole("link", { name: DIALS }));
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Busy",
    );

    rerender(page(<p>Another screen</p>));
    expect(
      within(screen.getByRole("dialog", { name: "Log activity" })).getByRole(
        "textbox",
        { name: "Subject" },
      ),
    ).toHaveProperty("value", "Busy");
  });

  // A mouse press in Safari leaves the link unfocused, as fireEvent does here.
  it("hands focus back to the pressed link, focused or not", async () => {
    const user = userEvent.setup();
    const { link } = await mountHost();

    fireEvent.click(link);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(link);
  });

  it("files the saved call on the number's contact", async () => {
    const user = userEvent.setup();
    const { posted, link } = await mountHost();

    await user.click(link);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "No answer",
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "call",
      subject: "No answer",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });

  it("opens from the keyboard, takes focus, and records nothing when closed", async () => {
    const user = userEvent.setup();
    const { posted, link } = await mountHost();

    link.focus();
    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await waitFor(() =>
      expect(dialog.contains(document.activeElement)).toBe(true),
    );
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(link);
    expect(posted).toHaveLength(0);
  });

  it("only dials for a read seat, which may not log activity", async () => {
    const { link } = await mountHost("read");

    expect(fireEvent.click(link)).toBe(true);
    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    await expect(
      screen.findByRole("dialog", undefined, { timeout: 300 }),
    ).rejects.toThrow();
  });
});

// A dialog logs its own contact's call in place and hands any other on.
function TaskLike({ contactId }: Readonly<{ contactId: string }>) {
  const { call, start, close } = useCallLogFor(contactId);
  return (
    <CallStartProvider start={start}>
      <CallAction
        appearance="button"
        number={DIALS}
        label="Call own"
        record={DANA}
      />
      <CallAction
        appearance="button"
        number={DIALS}
        label="Call other"
        record={{ entityType: "contact", entityId: "p-2" }}
      />
      {call && <CallLogInline call={call} onClose={close} />}
    </CallStartProvider>
  );
}

function mountInline(outer: CallStart | null) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
  });
  render(
    <StoryProviders>
      <CallStartProvider start={outer}>
        <TaskLike contactId="p-1" />
      </CallStartProvider>
    </StoryProviders>,
  );
}

describe("a call started inside a surface that logs in place", () => {
  it("logs its own contact here and hands another contact's call on", () => {
    const outer = vi.fn();
    mountInline(outer);

    fireEvent.click(screen.getByRole("link", { name: "Call other" }));
    expect(outer).toHaveBeenCalledWith({
      entityType: "contact",
      entityId: "p-2",
    });
    expect(screen.queryByRole("region", { name: "Log activity" })).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: "Call own" }));
    expect(outer).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("region", { name: "Log activity" })).toBeTruthy();
  });

  it("only dials where nothing above it logs", async () => {
    mountInline(null);

    fireEvent.click(screen.getByRole("link", { name: "Call own" }));
    await expect(
      screen.findByRole("region", undefined, { timeout: 300 }),
    ).rejects.toThrow();
  });

  it("brings focus back to the open form on a second press", async () => {
    const user = userEvent.setup();
    mountInline(vi.fn());
    const own = screen.getByRole("link", { name: "Call own" });

    fireEvent.click(own);
    const log = screen.getByRole("region", { name: "Log activity" });
    await waitFor(() =>
      expect(log.contains(document.activeElement)).toBe(true),
    );
    await user.type(
      within(log).getByRole("textbox", { name: "Subject" }),
      "Busy",
    );
    own.focus();
    fireEvent.click(own);

    await waitFor(() =>
      expect(log.contains(document.activeElement)).toBe(true),
    );
    expect(
      within(log).getByRole("textbox", { name: "Subject" }),
    ).toHaveProperty("value", "Busy");
  });

  it("asks before Close throws away typed words, and keeps them on Cancel", async () => {
    const user = userEvent.setup();
    mountInline(vi.fn());

    fireEvent.click(screen.getByRole("link", { name: "Call own" }));
    const log = screen.getByRole("region", { name: "Log activity" });
    await user.type(
      within(log).getByRole("textbox", { name: "Subject" }),
      "Busy",
    );
    await user.click(within(log).getByRole("button", { name: "Close" }));

    const question = await screen.findByRole("dialog", {
      name: "Discard unsaved changes?",
    });
    await user.click(within(question).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("region", { name: "Log activity" })).toBeTruthy();

    await user.click(within(log).getByRole("button", { name: "Close" }));
    await user.click(
      within(
        await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
      ).getByRole("button", { name: "Discard changes" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Log activity" })).toBeNull(),
    );
  });
});
