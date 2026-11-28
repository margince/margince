/** @vitest-environment happy-dom */
import { en as englishCopy } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { MessengerMessages } from "./messenger.messages";
import { useMessengerConversation } from "./messenger.queries";
import { messengerContext, messengerScope } from "./messenger.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

function ConversationHost() {
  const current = useMessengerConversation(messengerScope, "C_PILOT", true);
  return (
    current.data && (
      <MessengerMessages
        scope={messengerScope}
        context={current.data}
        seat="synthetic-seat"
      />
    )
  );
}

it("refreshes the actual recipients and requires review before writing to a changed audience", async () => {
  let context = messengerContext;
  const json = (body: unknown) =>
    new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path === "/v1/me")
        return json(meFixture({ allow: {}, seat: "full" }));
      if (path.endsWith("/conversation")) return json(context);
      if (path.endsWith("/messages"))
        return json({ messages: [], next_cursor: "" });
      throw new Error(`Unexpected route ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConversationHost />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("textbox")).toBeVisible();
  context = {
    ...messengerContext,
    audience_key: "changed-roster",
    conversation: {
      ...messengerContext.conversation,
      members: [
        ...messengerContext.conversation.members,
        { id: "U_NEW", name: "Sam Rivera", active: true, external: false },
      ],
    },
  };
  await user.click(
    screen.getByRole("button", { name: englishCopy["messenger.refresh"] }),
  );
  expect(
    await screen.findByText(englishCopy["messenger.audienceChanged"]),
  ).toBeVisible();
  await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["messenger.reviewAudience"],
    }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("Sam Rivera")).toBeVisible();
  await user.click(
    within(dialog).getByRole("button", {
      name: englishCopy["messenger.useAudience"],
    }),
  );
  expect(await screen.findByRole("textbox")).toBeVisible();
  client.clear();
});

it("loads later thread replies in order and advances the returned cursor on every page", async () => {
  const cursors: string[] = [];
  const json = (body: unknown) =>
    new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  const message = (id: string, text: string, at: string) => ({
    id,
    text,
    at,
    author: "U_JORDAN",
    root_id: "",
    edited: false,
    deleted: false,
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname === "/v1/me")
        return json(meFixture({ allow: {}, seat: "full" }));
      if (url.pathname.endsWith("/conversation")) return json(messengerContext);
      if (url.pathname.endsWith("/messages")) {
        if (!url.searchParams.get("root_id"))
          return json({
            messages: [
              message("ROOT", "Start the review", "2026-06-01T02:00:00Z"),
            ],
            next_cursor: "",
          });
        const cursor = url.searchParams.get("cursor") ?? "";
        if (!cursor)
          return json({
            messages: [
              message("ONE", "First reply", "2026-06-01T10:00:00+07:00"),
            ],
            next_cursor: "later-1",
            cursor_direction: "newer",
          });
        cursors.push(cursor);
        if (cursor === "later-1")
          return json({
            messages: [message("TWO", "Second reply", "2026-06-01T04:00:00Z")],
            next_cursor: "later-2",
            cursor_direction: "newer",
          });
        if (cursor === "later-2")
          return json({
            messages: [message("THREE", "Third reply", "2026-06-01T05:00:00Z")],
            next_cursor: "",
            cursor_direction: "newer",
          });
      }
      throw new Error(`Unexpected route ${url}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConversationHost />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(
    await screen.findByRole("button", { name: englishCopy["messenger.reply"] }),
  );
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.loadLater"],
    }),
  );
  expect(await screen.findByText("Second reply")).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: englishCopy["messenger.loadLater"] }),
  );
  expect(await screen.findByText("Third reply")).toBeVisible();
  expect(cursors).toEqual(["later-1", "later-2"]);
  expect(
    screen
      .getAllByRole("article")
      .map((row) => row.querySelector(".messenger-message-text")?.textContent),
  ).toEqual(["First reply", "Second reply", "Third reply"]);
  expect(
    screen.queryByRole("button", {
      name: englishCopy["messenger.loadEarlier"],
    }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: englishCopy["messenger.loadLater"] }),
  ).toBeNull();
  client.clear();
});
