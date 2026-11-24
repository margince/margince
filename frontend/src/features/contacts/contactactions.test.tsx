// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { armHoverIntent } from "../../design-system/hoverintent-testing";
import { en } from "../../i18n/en";
import { useShortcutDispatcher } from "../../lib/shortcuts";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactActions } from "./contactactions";

// The verb shows "Meet", so its name starts with the word a speech user says.
const MEET_NAME = `${en["record.verb.meet"]}, ${en["contact.action.meetings"]}`;

afterEach(cleanup);

// Reachable both ways, so a header that drew a Call verb would show it.
const reachable: components["schemas"]["Contact360"] = {
  ...view,
  contact: {
    ...view.contact,
    phones: [
      {
        id: "ph-0",
        phone: "+49 30 1111 2222",
        phone_type: "work",
        is_primary: true,
        position: 0,
        source: "manual",
        captured_by: "human:u-1",
      },
    ],
  },
};

type Verbs = Readonly<{
  onWrite: () => void;
  onLogActivity: () => void;
  onAddTask: () => void;
}>;

const noVerbs: Verbs = {
  onWrite: () => undefined,
  onLogActivity: () => undefined,
  onAddTask: () => undefined,
};

function mountHeader(
  contact = reachable.contact,
  verbs = noVerbs,
  writeRefusalId?: string,
) {
  installFetchStub({
    "GET /me": meRoute(
      { contact: ["read", "update"], activity: ["create"] },
      { seat: "full" },
    ),
    "GET /channel-providers": () => jsonResponse({ data: [] }),
  });
  render(
    <StoryProviders>
      <ContactActions
        view={{ ...reachable, contact }}
        contactId={reachable.contact.id}
        onResearch={() => undefined}
        writeRefusalId={writeRefusalId}
        {...verbs}
      />
      {writeRefusalId && (
        <p id={writeRefusalId}>{en["contact.action.noTransport"]}</p>
      )}
    </StoryProviders>,
  );
}

describe("the contact header", () => {
  it("leads with Email and leaves dialling to the number", () => {
    mountHeader();
    expect(
      screen.getByRole("button", { name: en["contact.action.email"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: en["contact.action.call"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["contact.action.call"] }),
    ).toBeNull();
  });

  it("keeps meetings, Log activity and the menu, with Add task inside it", () => {
    mountHeader();
    expect(
      screen.queryByRole("button", { name: en["contact.action.addTask"] }),
    ).toBeNull();
    for (const verb of [MEET_NAME, en["log.title"], en["record.moreActions"]]) {
      expect(screen.getByRole("button", { name: verb })).toBeTruthy();
    }
  });
});

it.each([false, undefined])(
  "refuses booking and logging without an attach right: %s",
  async (attachable) => {
    const user = userEvent.setup();
    mountHeader({ ...reachable.contact, attachable });
    const reason = await screen.findByText(en["contact.attachRefused"]);
    const log = screen.getByRole("button", { name: en["log.title"] });
    expect(log).toHaveProperty("disabled", true);
    expect(log.getAttribute("aria-describedby")).toBe(reason.id);
    await user.click(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    );
    const book = screen.getByRole("button", { name: en["scheduling.new"] });
    expect(book).toHaveProperty("disabled", true);
    expect(book.getAttribute("aria-describedby")).toBe(reason.id);
    expect(
      screen.getByRole("button", { name: en["contact.action.addTask"] }),
    ).toHaveProperty("disabled", true);
  },
);

it("allows attaching to a readable contact even when its row cannot be edited", async () => {
  const user = userEvent.setup();
  mountHeader({ ...reachable.contact, writable: false, attachable: true });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: en["log.title"] }),
    ).toHaveProperty("disabled", false),
  );
  await user.click(
    screen.getByRole("button", { name: en["record.moreActions"] }),
  );
  expect(
    screen.getByRole("button", { name: en["scheduling.new"] }),
  ).toHaveProperty("disabled", false);
});

describe("the record keys", () => {
  const spies = () => ({
    onWrite: vi.fn(),
    onLogActivity: vi.fn(),
    onAddTask: vi.fn(),
  });

  it("run the verbs the header's own buttons run", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    const verbs = spies();
    mountHeader(reachable.contact, verbs);
    const log = screen.getByRole("button", { name: en["log.title"] });
    await waitFor(() => expect(log).toHaveProperty("disabled", false));
    expect(log.getAttribute("aria-keyshortcuts")).toBe("n");
    expect(
      screen
        .getByRole("button", { name: en["contact.action.email"] })
        .getAttribute("aria-keyshortcuts"),
    ).toBe("m");

    await user.keyboard("n");
    expect(verbs.onLogActivity).toHaveBeenCalledOnce();
    await user.keyboard("t");
    expect(verbs.onAddTask).toHaveBeenCalledOnce();
    await user.keyboard("m");
    expect(verbs.onWrite).toHaveBeenCalledOnce();
  });

  it("do nothing on a contact the seat may not log against", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    const verbs = spies();
    mountHeader({ ...reachable.contact, attachable: false }, verbs);
    await screen.findByText(en["contact.attachRefused"]);

    await user.keyboard("ntm");
    expect(verbs.onLogActivity).not.toHaveBeenCalled();
    expect(verbs.onAddTask).not.toHaveBeenCalled();
    expect(verbs.onWrite).not.toHaveBeenCalled();
  });

  it("leave M alone on a contact with nowhere to write to", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    const verbs = spies();
    mountHeader({ ...reachable.contact, primary_email: null }, verbs);
    const log = screen.getByRole("button", { name: en["log.title"] });
    await waitFor(() => expect(log).toHaveProperty("disabled", false));

    await user.keyboard("mn");
    expect(verbs.onWrite).not.toHaveBeenCalled();
    expect(verbs.onLogActivity).toHaveBeenCalledOnce();
  });
});

describe("a refused verb on a contact with nowhere to write to", () => {
  const WHY = en["contact.action.noTransport"];
  const noAddress = { ...reachable.contact, emails: [], primary_email: null };

  it("says why on hover and is described once, by the page's sentence", async () => {
    const user = userEvent.setup();
    armHoverIntent();
    mountHeader(noAddress, noVerbs, "write-refused");
    const write = screen.getByRole("button", {
      name: en["contact.action.write"],
    });
    expect(write).toHaveProperty("disabled", true);
    expect(write.getAttribute("aria-describedby")).toBe("write-refused");
    expect(screen.getAllByText(WHY)).toHaveLength(1);

    const host = write.closest(".icon-action");
    if (!(host instanceof HTMLElement)) {
      throw new Error("the Email verb lost its tooltip host");
    }
    await user.hover(host);
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      `${en["contact.action.write"]}. ${WHY}`,
    );
    expect(write.getAttribute("aria-describedby")).toBe("write-refused");
  });

  it("names the log refusal in Log's tip, outside its description", async () => {
    mountHeader({ ...reachable.contact, attachable: false });
    const reason = await screen.findByText(en["contact.attachRefused"]);
    const log = screen.getByRole("button", { name: en["log.title"] });
    expect(log.getAttribute("title")).toBe(
      `${en["log.title"]}. ${en["contact.attachRefused"]}`,
    );
    expect(log.getAttribute("aria-describedby")).toBe(reason.id);
  });
});
