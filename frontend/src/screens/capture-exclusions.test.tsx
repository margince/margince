/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { CaptureExclusionsCard } from "./capture-exclusions";
import {
  backend,
  CAPTURE_EDITOR,
  openForm,
  Providers,
  READER,
  RULES,
  removeVerb,
} from "./capture-exclusions.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

describe("the deletion receipt", () => {
  function backendWithPurge(outcome: Record<string, unknown>) {
    const { fetchMock, calls } = backend(CAPTURE_EDITOR);
    const wrapped = async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.includes("/purge")) {
        calls.push({
          method: request.method,
          url: request.url,
          body: undefined,
        });
        return new Response(JSON.stringify(outcome), {
          headers: { "Content-Type": "application/json" },
        });
      }
      return fetchMock(input, init);
    };
    return { wrapped, calls };
  }

  async function openPurge(user: ReturnType<typeof userEvent.setup>) {
    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getAllByRole("button", {
        name: /Delete mail already captured/,
      })[0],
    );
  }

  // The promise the information sheet makes is about the owner's own data, so
  // the owner has to be able to check it. Nothing destroys without a look
  // first: the act is irreversible.
  it("shows what would go before anything goes", async () => {
    const user = userEvent.setup();
    const { wrapped, calls } = backendWithPurge({
      destroyed: 3,
      released: 0,
      skipped: 0,
      anonymised: 0,
      preview: true,
      kept: { held: 0, under_statute: 0, under_request: 0 },
    });
    vi.stubGlobal("fetch", wrapped);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await openPurge(user);
    await user.click(screen.getByRole("button", { name: "Check first" }));

    expect(
      await screen.findByText(/3 messages would be destroyed/),
    ).toBeTruthy();
    // The preview asked for a preview, and destroyed nothing.
    expect(calls.some((call) => call.url.includes("preview=true"))).toBe(true);
    expect(calls.some((call) => call.url.includes("preview=false"))).toBe(
      false,
    );
  });

  // The half that matters most. A deletion that correctly leaves a Handelsbrief
  // standing looks, from the owner's side, exactly like one that silently
  // failed — so the reason and the period are named.
  it("says what the law kept, and for how long", async () => {
    const user = userEvent.setup();
    const { wrapped } = backendWithPurge({
      destroyed: 1,
      released: 0,
      skipped: 2,
      anonymised: 0,
      preview: true,
      kept: {
        held: 0,
        under_statute: 2,
        under_request: 0,
        statutory_class: "commercial_correspondence",
        statutory_years: 6,
        statutory_from_year_end: true,
      },
    });
    vi.stubGlobal("fetch", wrapped);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await openPurge(user);
    await user.click(screen.getByRole("button", { name: "Check first" }));

    const kept = await screen.findByText(/kept as commercial correspondence/);
    // The years a reader can act on, and the anchor that makes six years mean
    // up to seven. A raw ISO duration here would be a machine's spelling.
    expect(kept.textContent).toContain("6 years");
    expect(kept.textContent).toContain("end of the calendar year");
    expect(kept.textContent).not.toContain("P6Y");
  });

  // The three reasons lift on different days, so they are different sentences.
  it("does not report a hold as a statutory period", async () => {
    const user = userEvent.setup();
    const { wrapped } = backendWithPurge({
      destroyed: 0,
      released: 0,
      skipped: 1,
      anonymised: 0,
      preview: true,
      kept: { held: 1, under_statute: 0, under_request: 0 },
    });
    vi.stubGlobal("fetch", wrapped);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await openPurge(user);
    await user.click(screen.getByRole("button", { name: "Check first" }));

    expect(await screen.findByText(/is pinned/)).toBeTruthy();
    expect(screen.queryByText(/commercial correspondence/)).toBeNull();
  });

  // Escape mid-purge would unmount the dialog and lose the receipt the server
  // is about to send back.
  it("stays open on Escape while the purge is running", async () => {
    const user = userEvent.setup();
    const { wrapped } = backendWithPurge({
      destroyed: 3,
      released: 0,
      skipped: 0,
      anonymised: 0,
      preview: true,
      kept: { held: 0, under_statute: 0, under_request: 0 },
    });
    let answer: () => void = () => {};
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/purge")) await answered;
        return wrapped(input, init);
      },
    );
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await openPurge(user);
    await user.click(screen.getByRole("button", { name: "Check first" }));
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();

    answer();
    expect(
      await screen.findByText(/3 messages would be destroyed/),
    ).toBeTruthy();
  });
});

// The third reason, which renders on its own branch: a request still being
// answered needs the mail to answer with, and lifts on a different day from
// a pin or a retention window.
it("names an open request as the reason mail was kept", async () => {
  const user = userEvent.setup();
  const { fetchMock } = backend(CAPTURE_EDITOR);
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.includes("/purge")) {
        return new Response(
          JSON.stringify({
            destroyed: 0,
            released: 0,
            skipped: 1,
            anonymised: 0,
            preview: true,
            kept: { held: 0, under_statute: 0, under_request: 1 },
          }),
          { headers: { "Content-Type": "application/json" } },
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

  await waitFor(() => expect(screen.getByText("ex@partner.test")).toBeTruthy());
  await user.click(
    screen.getAllByRole("button", { name: /Delete mail already captured/ })[0],
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  expect(
    await screen.findByText(/data-protection request is still being answered/),
  ).toBeTruthy();
});

// Confirming is the irreversible half, and it only becomes available after a
// look. The receipt then reports what WAS destroyed rather than what would be.
it("destroys only after the preview, and reports it in the past tense", async () => {
  const user = userEvent.setup();
  const { fetchMock } = backend(CAPTURE_EDITOR);
  const purgeCalls: string[] = [];
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.includes("/purge")) {
        const preview = request.url.includes("preview=true");
        purgeCalls.push(request.url);
        return new Response(
          JSON.stringify({
            destroyed: 2,
            released: 1,
            skipped: 0,
            anonymised: 1,
            preview,
            kept: { held: 0, under_statute: 0, under_request: 0 },
          }),
          { headers: { "Content-Type": "application/json" } },
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

  await waitFor(() => expect(screen.getByText("ex@partner.test")).toBeTruthy());
  await user.click(
    screen.getAllByRole("button", { name: /Delete mail already captured/ })[0],
  );

  // Nothing to confirm until a look has been taken.
  expect(
    screen.queryByRole("button", { name: "Delete permanently" }),
  ).toBeNull();

  await user.click(screen.getByRole("button", { name: "Check first" }));
  expect(await screen.findByText(/2 messages would be destroyed/)).toBeTruthy();

  await user.click(screen.getByRole("button", { name: "Delete permanently" }));
  expect(await screen.findByText(/2 messages destroyed/)).toBeTruthy();
  // The colleague's copy and the stripped contact are reported too.
  expect(screen.getByText(/Your access to it has ended/)).toBeTruthy();
  expect(screen.getByText(/stripped of identifying details/)).toBeTruthy();
  expect(purgeCalls.some((url) => url.includes("preview=false"))).toBe(true);
});

// Without the year-end anchor the promise is plainly the period itself, and
// saying otherwise would overstate how long somebody's mail is held.
it("drops the year-end qualifier when the period does not carry one", async () => {
  const user = userEvent.setup();
  const { fetchMock } = backend(CAPTURE_EDITOR);
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.includes("/purge")) {
        return new Response(
          JSON.stringify({
            destroyed: 0,
            released: 0,
            skipped: 1,
            anonymised: 0,
            preview: true,
            kept: {
              held: 0,
              under_statute: 1,
              under_request: 0,
              statutory_class: "commercial_correspondence",
              statutory_years: 2,
              statutory_from_year_end: false,
            },
          }),
          { headers: { "Content-Type": "application/json" } },
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

  await waitFor(() => expect(screen.getByText("ex@partner.test")).toBeTruthy());
  await user.click(
    screen.getAllByRole("button", { name: /Delete mail already captured/ })[0],
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  const kept = await screen.findByText(/kept as commercial correspondence/);
  expect(kept.textContent).toContain("2 years");
  expect(kept.textContent).not.toContain("end of the calendar year");
});

// A period the packs declare in months or days has no number this copy could
// say truthfully, so the class is named and the period is not.
it("names the class without a period it cannot state in years", async () => {
  const user = userEvent.setup();
  const { fetchMock } = backend(CAPTURE_EDITOR);
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.includes("/purge")) {
        return new Response(
          JSON.stringify({
            destroyed: 0,
            released: 0,
            skipped: 1,
            anonymised: 0,
            preview: true,
            kept: {
              held: 0,
              under_statute: 1,
              under_request: 0,
              statutory_class: "commercial_correspondence",
              statutory_from_year_end: false,
            },
          }),
          { headers: { "Content-Type": "application/json" } },
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

  await waitFor(() => expect(screen.getByText("ex@partner.test")).toBeTruthy());
  await user.click(
    screen.getAllByRole("button", { name: /Delete mail already captured/ })[0],
  );
  await user.click(screen.getByRole("button", { name: "Check first" }));

  const kept = await screen.findByText(/kept as commercial correspondence/);
  expect(kept.textContent).not.toContain("The law requires keeping");
});
