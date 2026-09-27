/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CaptureExclusionsCard } from "./capture-exclusions";

// The two scopes this card draws are two different permissions, and that is the
// whole subject: a rule that binds only the reader is theirs to write, while one
// that binds the company is admin/ops work. So every case below fixes the
// grant and asks what the card offers.
const CAPTURE_EDITOR: GrantSpec = { capture_settings: ["read", "update"] };
const READER: GrantSpec = { capture_settings: ["read"] };

const RULES = [
  { id: "cx-1", scope: "user", kind: "address", value: "ex@partner.test" },
  { id: "cx-2", scope: "workspace", kind: "domain", value: "recruiter.test" },
];

type Call = { method: string; url: string; body: unknown };

function backend(
  allow: GrantSpec,
  rules: unknown[] = RULES,
  connectedProvider = "gmail",
) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      // Built as a Request rather than read off `init`: openapi-fetch may pass a
      // Request with no init at all, and a mock that read the method from init
      // would answer every write as if it were a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = request.url;
      const method = request.method;
      calls.push({
        method,
        url,
        body: method === "GET" ? undefined : await request.clone().json(),
      });
      if (url.endsWith("/v1/me")) {
        return new Response(JSON.stringify(meFixture({ allow })), {
          headers: { "Content-Type": "application/json" },
        });
      }
      // The seat's own connections: the picker asks WHICH mailbox's folders
      // it is offering, so a fixture without one offers nothing.
      if (url.includes("/connectors") && !url.includes("/containers")) {
        return new Response(
          JSON.stringify({
            data: [
              {
                id: "conn-1",
                provider: connectedProvider,
                status: "connected",
                scopes: [],
              },
            ],
            providers: [],
          }),
          { headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.includes("/containers")) {
        return new Response(
          JSON.stringify({
            containers: url.includes("/graph/")
              ? [{ id: "AAMk-privat", name: "Posteingang/Privat" }]
              : [{ id: "Label_7", name: "Privat" }],
          }),
          { headers: { "Content-Type": "application/json" } },
        );
      }
      if (method === "POST") {
        return new Response(JSON.stringify({ id: "cx-3" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ data: rules }), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  return { fetchMock, calls };
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The header verb that opens the form, named by its catalog key: the dialog's
// own submit says "Exclude", so matching on that would find the wrong control.
const openForm = () =>
  screen.getByRole("button", { name: en["captureExclusions.addOpen"] });

const removeVerb = (value: string) =>
  screen.getByRole("button", {
    name: en["captureExclusions.remove"].replace("{value}", value),
  }) as HTMLButtonElement;

describe("CaptureExclusionsCard", () => {
  it("lists each rule with the scope and kind it binds by", async () => {
    const { fetchMock } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    // The answer beside each rule is what it binds and what kind it is — the
    // two facts that decide whether this reader may take it back.
    expect(screen.getByText("Your mailboxes · Address")).toBeTruthy();
    expect(screen.getByText("Whole company · Domain")).toBeTruthy();
  });

  // The permission split, on the row: a reader may take back their own rule and
  // may not take back the company's, and the refusal names one sentence
  // rather than printing it per row.
  it("refuses only the company-wide rule to a seat without the update grant", async () => {
    const { fetchMock } = backend(READER);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("recruiter.test")).toBeTruthy(),
    );
    expect(removeVerb("ex@partner.test").disabled).toBe(false);
    const refused = removeVerb("recruiter.test");
    expect(refused.disabled).toBe(true);
    // Pointed at, not repeated: the button names the id of the one sentence on
    // the card that says why.
    const reason = refused.getAttribute("aria-describedby");
    expect(reason).toBeTruthy();
    expect(document.getElementById(reason ?? "")?.textContent).toBe(
      en["captureSettings.adminOnly"],
    );
  });

  // The sentence is a claim about this reader, so it is only made when a row on
  // the card actually bears it out.
  it("says nothing about permissions when no rule on the card is refused", async () => {
    const { fetchMock } = backend(READER, [RULES[0]]);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    expect(screen.queryByText(en["captureSettings.adminOnly"])).toBeNull();
  });

  it("reads as empty rather than as a failed read when nothing is excluded", async () => {
    const { fetchMock } = backend(CAPTURE_EDITOR, []);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByTestId("capture-exclusions-empty").textContent).toBe(
        en["captureExclusions.empty"],
      ),
    );
    // Anybody may keep their OWN correspondent out, so the verb that opens the
    // form is offered even here — the dialog refuses the scope, not the card.
    expect(openForm()).toBeTruthy();
  });

  it("sends the scope, kind and value the form was filled with", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(openForm());
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.kind.domain"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["captureExclusions.addLabel"] }),
      "  jobs.test  ",
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.add"] }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    const posted = calls.find((call) => call.method === "POST");
    // Trimmed: a value with a stray space is a rule that matches nothing, and
    // the reader cannot see the difference.
    expect(posted?.body).toEqual({
      scope: "user",
      kind: "domain",
      value: "jobs.test",
    });
  });

  // The scope a seat may not write is refused where that choice is made, rather
  // than by a card that offered the form and then rejected the submission.
  it("refuses the company scope inside the dialog", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(READER, [RULES[0]]);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(openForm());
    const value = screen.getByRole("textbox", {
      name: en["captureExclusions.addLabel"],
    }) as HTMLInputElement;
    await user.type(value, "jobs.test");
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    );

    expect(value.disabled).toBe(true);
    const submit = screen.getByRole("button", {
      name: en["captureExclusions.add"],
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await user.click(submit);
    expect(calls.some((call) => call.method === "POST")).toBe(false);
  });
});

describe("the container kind", () => {
  // The whole point of the picker: a rule names a provider's own token, and
  // the reader chooses a name they recognise.
  it("offers the mailbox's folders by name and stores the provider's token", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    // The NAME is what the reader sees and picks.
    const picker = await screen.findByRole("combobox");
    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: "Privat" }));
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.add"] }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    const posted = calls.find((call) => call.method === "POST");
    // The provider-qualified TOKEN is stored, never the display name: a
    // rename must not silently re-point a rule. And the scope is the
    // reader's own, because a label means nothing in anybody else's mailbox.
    expect(posted?.body).toMatchObject({
      kind: "container",
      value: "gmail:Label_7",
      scope: "user",
    });
  });

  // A mailbox that reports no folders is told so, rather than left with a
  // picker that looks broken. The server refuses a container rule it has no
  // token for anyway, so an empty picker would be a dead control.
  it("says so when the mailbox reports no folders", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    // Answer the containers read with an empty list.
    const empty = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.url.includes("/containers")) {
          return new Response(JSON.stringify({ containers: [] }), {
            headers: { "Content-Type": "application/json" },
          });
        }
        return fetchMock(input, init);
      },
    );
    vi.stubGlobal("fetch", empty);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    expect(
      await screen.findByText(en["captureExclusions.noContainers"]),
    ).toBeTruthy();
  });

  // A label lives in ONE mailbox, so there is no workspace choice to offer —
  // the database refuses that rule, and showing the control would offer a
  // scope the write cannot take.
  it("offers no scope choice, because a label is the reader's own", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    // The scope control is there for an address rule...
    expect(
      screen.queryByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );
    // ...and gone for a container one.
    expect(
      screen.queryByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    ).toBeNull();
  });
});

describe("the picker follows the seat's own mailbox", () => {
  // The picker used to ask Gmail whatever the reader had connected, so an
  // Outlook or IMAP seat opened it, was answered 404, and saw "no folders" —
  // the one kind that exists to save them typing a provider token.
  it("asks the connected provider, not a fixed one", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(CAPTURE_EDITOR, RULES, "graph");
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    await user.click(await screen.findByRole("combobox"));
    await user.click(
      await screen.findByRole("option", { name: "Posteingang/Privat" }),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.add"] }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    expect(
      calls.some((call) => call.url.includes("/connectors/graph/containers")),
    ).toBe(true);
    expect(
      calls.some((call) => call.url.includes("/connectors/gmail/containers")),
    ).toBe(false);
    // Qualified by the provider that actually answered.
    expect(calls.find((call) => call.method === "POST")?.body).toMatchObject({
      kind: "container",
      value: "graph:AAMk-privat",
    });
  });

  // A provider that did not answer is not a mailbox with no folders, and the
  // two sentences send a reader somewhere different.
  it("says a failed read is a failure, not an empty mailbox", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.url.includes("/containers")) {
          return new Response(
            JSON.stringify({
              code: "provider_unreachable",
              title: "no answer",
            }),
            { status: 502, headers: { "Content-Type": "application/json" } },
          );
        }
        return fetchMock(input, init);
      },
    );
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    await waitFor(() =>
      expect(
        screen.getByText(en["captureExclusions.containersUnreadable"]),
      ).toBeTruthy(),
    );
    expect(screen.queryByText(en["captureExclusions.noContainers"])).toBeNull();
  });

  // The text box and the picker share one draft. An address typed and then
  // left behind by a switch to the container kind used to pass the submit
  // guard and fail at the store.
  it("drops a draft that the new kind cannot mean", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["captureExclusions.addLabel"] }),
      "someone@example.com",
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.add"] }),
    );
    // Nothing was carried across, so nothing was submitted.
    expect(calls.some((call) => call.method === "POST")).toBe(false);
  });

  // A container rule is forced to the reader's own scope, so the company-scope
  // refusal must not follow them into it. Choosing "whole company" for an
  // address and then switching to the folder picker used to leave the picker
  // disabled for a rule that binds nobody but the reader.
  it("does not carry the company-scope refusal into the folder picker", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(READER, [RULES[0]]);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    const picker = (await screen.findByRole("combobox")) as HTMLSelectElement;
    expect(picker.disabled).toBe(false);
  });
});

// A list of connections that could not be read is not a seat with no mailbox.
// Same distinction as a failed folder read, one layer up.
it("says so when the connections themselves cannot be read", async () => {
  const user = userEvent.setup();
  const { fetchMock } = backend(CAPTURE_EDITOR);
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (
        request.url.includes("/connectors") &&
        !request.url.includes("/containers")
      ) {
        return new Response(JSON.stringify({ title: "no answer" }), {
          status: 502,
          headers: { "Content-Type": "application/json" },
        });
      }
      return fetchMock(input, init);
    },
  );
  render(
    <Providers>
      <CaptureExclusionsCard />
    </Providers>,
  );

  await waitFor(() => expect(screen.getByText("ex@partner.test")).toBeTruthy());
  await user.click(
    screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
  );
  await user.click(
    screen.getByRole("button", {
      name: en["captureExclusions.kind.container"],
    }),
  );

  await waitFor(() =>
    expect(
      screen.getByText(en["captureExclusions.containersUnreadable"]),
    ).toBeTruthy(),
  );
  expect(screen.queryByText(en["captureExclusions.noContainers"])).toBeNull();
});
