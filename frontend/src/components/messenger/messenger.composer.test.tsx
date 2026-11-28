/** @vitest-environment happy-dom */
import { en as englishCopy } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { MessengerComposer } from "./messenger.composer";
import { keepMessengerDraft, readMessengerDraft } from "./messenger.draft";
import { messengerContext, messengerScope } from "./messenger.testkit";

function mount(key = "test-draft", context = messengerContext) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MessengerComposer
          scope={messengerScope}
          context={context}
          root=""
          draftKey={key}
          onSent={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, client };
}
function attemptResponse(
  request: string,
  state: "accepted" | "unknown" | "failed",
) {
  const attempt: components["schemas"]["MessengerAttempt"] = {
    id: "018f3a1b-0000-7000-8000-00000000a001",
    request_id: request,
    conversation_id: "C_PILOT",
    provider_message_id: state === "accepted" ? "123.456" : "",
    state,
    updated_at: "2026-06-01T10:00:00Z",
  };
  return new Response(JSON.stringify(attempt), {
    headers: { "content-type": "application/json" },
  });
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
  vi.useRealTimers();
});
function provider(state: "accepted" | "unknown" | "failed") {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me"))
        return new Response(JSON.stringify(meFixture({ allow: {} })), {
          headers: { "content-type": "application/json" },
        });
      requests.push(request);
      const body: unknown =
        request.method === "POST" ? await request.clone().json() : null;
      const requestID =
        typeof body === "object" &&
        body !== null &&
        "request_id" in body &&
        typeof body.request_id === "string"
          ? body.request_id
          : new URL(request.url).pathname.split("/").at(-1);

      return attemptResponse(requestID ?? "", state);
    }),
  );
  return requests;
}
it("sends desktop Enter with the reviewed audience and clears only an accepted draft", async () => {
  const requests = provider("accepted");
  const user = userEvent.setup();
  mount();
  await user.type(
    screen.getByRole("textbox"),
    "Can we confirm the pilot date?",
  );
  await user.keyboard("{Enter}");
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent(
      englishCopy["messenger.accepted"],
    ),
  );
  expect(requests).toHaveLength(1);
  const request = requests[0];
  expect(request.method).toBe("POST");
  expect(await request.clone().json()).toMatchObject({
    text: "Can we confirm the pilot date?",
    audience_key: "synthetic-roster",
    grant_version: 1,
  });
  expect(screen.getByRole("textbox")).toHaveValue("");
  expect(readMessengerDraft("test-draft").request).toBeNull();
});
it("keeps Shift+Enter and IME confirmation out of the send path", async () => {
  const requests = provider("accepted");
  const user = userEvent.setup();
  mount();
  await user.type(screen.getByRole("textbox"), "Line one");
  await user.keyboard("{Shift>}{Enter}{/Shift}");
  fireEvent.keyDown(screen.getByRole("textbox"), {
    key: "Enter",
    isComposing: true,
    keyCode: 229,
  });
  expect(requests).toHaveLength(0);
  expect(screen.getByRole("textbox")).toHaveValue("Line one\n");
});
it("touch Return writes a newline and the explicit send button sends", async () => {
  const requests = provider("accepted");
  vi.stubGlobal("matchMedia", () => ({
    matches: true,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
  const user = userEvent.setup();
  mount();
  await user.type(screen.getByRole("textbox"), "Hello{Enter}Jordan");
  expect(requests).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(requests).toHaveLength(1));
  expect((await requests[0].clone().json()).text).toBe("Hello\nJordan");
});
it("keeps uncertain delivery and its request across a close and prevents a blind resend", async () => {
  const requests = provider("unknown");
  const user = userEvent.setup();
  const view = mount();
  await user.type(screen.getByRole("textbox"), "Approval requested");
  await user.click(screen.getByRole("button", { name: "Send" }));
  await screen.findByRole("button", {
    name: englishCopy["messenger.checkReceipt"],
  });
  expect(screen.getByRole("textbox")).toBeDisabled();
  view.unmount();
  mount();
  expect(screen.getByRole("textbox")).toHaveValue("Approval requested");
  expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  expect(requests).toHaveLength(1);
});
it("keeps independent destination drafts and preserves a definitely failed message for editing", async () => {
  const requests = provider("failed");
  const user = userEvent.setup();
  keepMessengerDraft("other-room", { text: "Private note", request: null });
  mount();
  await user.type(screen.getByRole("textbox"), "Need a decision");
  await user.click(screen.getByRole("button", { name: "Send" }));
  await screen.findByText(englishCopy["messenger.failed"]);
  expect(requests).toHaveLength(1);
  expect(screen.getByRole("textbox")).toHaveValue("Need a decision");
  expect(screen.getByRole("textbox")).toBeEnabled();
  expect(readMessengerDraft("other-room").text).toBe("Private note");
});

it("keeps a provider cooldown while editing and enables sending only when it ends", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
  const requests = provider("accepted");
  keepMessengerDraft("test-draft", {
    text: "Please confirm",
    request: null,
    retryAt: "2026-06-01T10:00:02Z",
  });
  mount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "Please confirm the date" },
  });
  expect(readMessengerDraft("test-draft").retryAt).toBe("2026-06-01T10:00:02Z");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  expect(requests).toHaveLength(0);
});

it("does not erase a later draft when an earlier send finishes after its composer closes", async () => {
  const delivery: { started: boolean; complete: () => void } = {
    started: false,
    complete: () => {
      throw new Error("send has not started");
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me"))
        return new Response(JSON.stringify(meFixture({ allow: {} })), {
          headers: { "content-type": "application/json" },
        });
      const body: unknown = await request.clone().json();
      if (
        typeof body !== "object" ||
        body === null ||
        !("request_id" in body) ||
        typeof body.request_id !== "string"
      )
        throw new Error("send request missing its ID");
      const id = body.request_id;
      return new Promise<Response>((resolve) => {
        delivery.complete = () => resolve(attemptResponse(id, "accepted"));
        delivery.started = true;
      });
    }),
  );
  const user = userEvent.setup();
  const view = mount();
  await user.type(screen.getByRole("textbox"), "First message");
  await user.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(delivery.started).toBe(true));
  view.unmount();
  keepMessengerDraft("test-draft", { text: "A later draft", request: null });
  delivery.complete();
  await waitFor(() =>
    expect(
      view.client
        .getMutationCache()
        .getAll()
        .some((mutation) => mutation.state.status === "success"),
    ).toBe(true),
  );
  expect(readMessengerDraft("test-draft").text).toBe("A later draft");
});

it("keeps delivery uncertain when an accepted receipt belongs to another request", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      new URL(request.url).pathname.endsWith("/me")
        ? new Response(JSON.stringify(meFixture({ allow: {} })), {
            headers: { "content-type": "application/json" },
          })
        : attemptResponse("018f3a1b-0000-7000-8000-00000000b999", "accepted"),
    ),
  );
  const user = userEvent.setup();
  mount();
  await user.type(screen.getByRole("textbox"), "Keep this draft");
  await user.click(screen.getByRole("button", { name: "Send" }));
  await screen.findByRole("button", {
    name: englishCopy["messenger.checkReceipt"],
  });
  expect(screen.getByRole("textbox")).toHaveValue("Keep this draft");
  expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
});

it("restores an unresolved server receipt after reconnect and requires explicit review", async () => {
  const requests = provider("accepted");
  const request = "018f3a1b-0000-7000-8000-00000000a009";
  mount("another-device", {
    ...messengerContext,
    grant_version: 2,
    unresolved_attempt: {
      id: request,
      request_id: request,
      conversation_id: messengerContext.conversation.id,
      provider_message_id: "",
      state: "unknown",
      updated_at: "2026-06-01T10:00:00Z",
    },
  });
  const user = userEvent.setup();
  expect(screen.getByRole("textbox")).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Edit draft" }));
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["messenger.checkedConversation"],
    }),
  );
  await user.type(screen.getByRole("textbox"), "A newly reviewed message");
  await user.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(requests).toHaveLength(1));
  expect(await requests[0].clone().json()).toMatchObject({
    acknowledged_request_id: request,
    grant_version: 2,
    text: "A newly reviewed message",
  });
});
