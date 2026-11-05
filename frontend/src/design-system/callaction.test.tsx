// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { telUri } from "../format/contacturi";
import { LocaleProvider } from "../i18n";
import { CallStartProvider } from "../lib/callstart";
import { INSTALLATION_SETTINGS_KEY } from "../lib/uploadlimit";
import { renderWithQueryClient } from "../testing/queryrender";
import { CallAction } from "./callaction";

const ALL_ON = { call_whatsapp: true, call_telegram: true, call_zalo: true };

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Every other request fails the test: a messenger link reads nothing else.
function answerSettings(answer: () => Response | Promise<Response>) {
  const fetchSpy = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (
      new URL(url, "http://localhost").pathname.endsWith(
        "/installation/settings",
      )
    ) {
      return answer();
    }
    throw new Error(`unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetchSpy);
  return fetchSpy;
}

const render = renderWithQueryClient;

beforeEach(() => {
  answerSettings(() => jsonResponse(ALL_ON));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DIALS = "+49 30 1234 5678";
const REFUSED = "ask reception";
const NO_PHONE = "No phone number on this contact.";
const DANA = { entityType: "contact", entityId: "p-1" } as const;

function describedTextOf(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

describe("CallAction dials through telUri", () => {
  it("draws the number as a link to its tel:", () => {
    render(<CallAction appearance="value" record={DANA} number={DIALS} />);
    const link = screen.getByRole("link", { name: DIALS });
    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    expect(link.getAttribute("target")).toBeNull();
  });

  it("draws the labelled verb as a link to its tel:", () => {
    render(
      <CallAction
        appearance="button"
        record={DANA}
        number={DIALS}
        label="Call"
      />,
    );
    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(DIALS));
  });

  it("draws the square glyph as a link to its tel:", () => {
    render(
      <CallAction
        appearance="icon"
        record={DANA}
        number={DIALS}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(DIALS));
  });
});

describe("CallAction starts the call's log", () => {
  it("hands every appearance's record to the start, and still dials", () => {
    const start = vi.fn();
    render(
      <CallStartProvider start={start}>
        <CallAction appearance="value" number={DIALS} record={DANA} />
        <CallAction
          appearance="button"
          number={DIALS}
          label="Call"
          record={DANA}
        />
        <CallAction
          appearance="icon"
          number={DIALS}
          label="Dial"
          reason={NO_PHONE}
          record={DANA}
        />
      </CallStartProvider>,
    );
    for (const name of [DIALS, "Call", "Dial"]) {
      expect(fireEvent.click(screen.getByRole("link", { name }))).toBe(true);
    }
    expect(start.mock.calls).toEqual([[DANA], [DANA], [DANA]]);
  });

  it.each(["ctrlKey", "metaKey", "shiftKey", "altKey"])(
    "leaves a %s press to the browser and opens no log",
    (key) => {
      const start = vi.fn();
      render(
        <CallStartProvider start={start}>
          <CallAction appearance="value" number={DIALS} record={DANA} />
        </CallStartProvider>,
      );
      const link = screen.getByRole("link", { name: DIALS });
      expect(fireEvent.click(link, { [key]: true })).toBe(true);
      expect(start).not.toHaveBeenCalled();
      fireEvent.click(link);
      expect(start).toHaveBeenCalledWith(DANA);
    },
  );

  it("only dials where nothing hosts a log", () => {
    render(<CallAction appearance="value" number={DIALS} record={DANA} />);
    const link = screen.getByRole("link", { name: DIALS });
    expect(fireEvent.click(link)).toBe(true);
    expect(link.getAttribute("href")).toBe(telUri(DIALS));
  });
});

describe("CallAction with nothing to dial", () => {
  it("keeps a refused number as plain text", () => {
    render(<CallAction appearance="value" record={DANA} number={REFUSED} />);
    expect(screen.getByText(REFUSED).tagName).toBe("SPAN");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("draws no verb for a refused number or no number", () => {
    const { container } = render(
      <>
        <CallAction
          appearance="button"
          record={DANA}
          number={REFUSED}
          label="Call"
        />
        <CallAction
          appearance="button"
          record={DANA}
          number={undefined}
          label="Call"
        />
      </>,
    );
    expect(container.childElementCount).toBe(0);
  });

  it("refuses the glyph with the caller's reason when there is no number", () => {
    render(
      <CallAction
        appearance="icon"
        record={DANA}
        number={undefined}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    const control = screen.getByRole("button", { name: "Call" });
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe(NO_PHONE);
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("refuses the glyph for a number telUri refuses", () => {
    render(
      <CallAction
        appearance="icon"
        record={DANA}
        number={REFUSED}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    expect(describedTextOf(screen.getByRole("button", { name: "Call" }))).toBe(
      NO_PHONE,
    );
  });

  it("lets the page's refusal win over a number on file", () => {
    render(
      <>
        <CallAction
          appearance="icon"
          record={DANA}
          number={DIALS}
          label="Call"
          reason={NO_PHONE}
          reasonId="archived"
        />
        <p id="archived">This contact is archived.</p>
      </>,
    );
    const control = screen.getByRole("button", { name: "Call" });
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe("This contact is archived.");
    expect(screen.queryByText(NO_PHONE)).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });
});

const CHATS = {
  WhatsApp: "https://wa.me/493012345678",
  Telegram: "https://t.me/+493012345678",
  Zalo: "https://zalo.me/493012345678",
} as const;

// The chats fold behind one glyph, so a reader opens it before any is drawn.
async function chatLinks(): Promise<HTMLElement[]> {
  fireEvent.click(await screen.findByRole("button", { name: "Open a chat" }));
  return screen.findAllByRole("link", { name: /^Open chat in / });
}

describe("CallAction offers the number's chat in each messenger", () => {
  it("draws one link per messenger behind the number's chat glyph, each off our origin", async () => {
    render(<CallAction appearance="value" record={DANA} number={DIALS} />);
    const links = await chatLinks();
    expect(
      links.map((link) => [link.textContent, link.getAttribute("href")]),
    ).toEqual(Object.entries(CHATS));
    for (const link of links) {
      expect(link.getAttribute("target")).toBe("_blank");
      expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    }
  });

  it.each([
    ["call_whatsapp", "WhatsApp"],
    ["call_telegram", "Telegram"],
    ["call_zalo", "Zalo"],
  ] as const)("drops only its own link when %s is off", async (key, name) => {
    answerSettings(() => jsonResponse({ ...ALL_ON, [key]: false }));
    render(<CallAction appearance="value" record={DANA} number={DIALS} />);
    const shown = (await chatLinks()).map((link) => link.textContent);
    expect(shown).toEqual(Object.keys(CHATS).filter((app) => app !== name));
  });

  it("draws no link before the settings answer, and all once they do", async () => {
    let answer: (response: Response) => void = () => {};
    answerSettings(
      () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    );
    render(<CallAction appearance="value" record={DANA} number={DIALS} />);
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.getByRole("link", { name: DIALS })).toBeTruthy();
    expect(screen.queryByRole("link", { name: /^Open chat in / })).toBeNull();
    answer(jsonResponse(ALL_ON));
    expect(await chatLinks()).toHaveLength(3);
  });

  it("falls back to every messenger when the seat may not read the settings", async () => {
    answerSettings(() =>
      jsonResponse(
        { title: "Forbidden", status: 403, code: "permission_denied" },
        403,
      ),
    );
    render(<CallAction appearance="value" record={DANA} number={DIALS} />);
    expect(await chatLinks()).toHaveLength(3);
  });

  it.each([
    ["the server fails", () => jsonResponse({ status: 500 }, 500)],
    [
      "the network fails",
      (): Response => {
        throw new TypeError("Failed to fetch");
      },
    ],
  ])(
    "offers no chat when %s, so a link turned off stays off",
    async (_, answer) => {
      answerSettings(answer);
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      rtlRender(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">
            <CallAction appearance="value" record={DANA} number={DIALS} />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      await vi.waitFor(() =>
        expect(client.getQueryState(INSTALLATION_SETTINGS_KEY)?.status).toBe(
          "error",
        ),
      );
      expect(screen.getByRole("link", { name: DIALS })).toBeTruthy();
      expect(screen.queryByRole("link", { name: /^Open chat in / })).toBeNull();
    },
  );

  it("offers no chat for a number without its country code, and asks nothing", () => {
    const fetchSpy = answerSettings(() => jsonResponse(ALL_ON));
    render(
      <CallAction appearance="value" record={DANA} number="030 1234 5678" />,
    );
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("opens a chat without starting the call's log", async () => {
    const start = vi.fn();
    render(
      <CallStartProvider start={start}>
        <CallAction appearance="value" record={DANA} number={DIALS} />
      </CallStartProvider>,
    );
    const [whatsapp] = await chatLinks();
    if (!whatsapp) throw new Error("no WhatsApp link");
    fireEvent.click(whatsapp);
    expect(start).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("link", { name: DIALS }));
    expect(start).toHaveBeenCalledWith(DANA);
  });

  it("keeps the verb and the glyph to the call alone", () => {
    const fetchSpy = answerSettings(() => jsonResponse(ALL_ON));
    const { container } = render(
      <>
        <CallAction
          appearance="button"
          record={DANA}
          number={DIALS}
          label="Call"
        />
        <CallAction
          appearance="icon"
          record={DANA}
          number={DIALS}
          label="Dial"
          reason={NO_PHONE}
        />
      </>,
    );
    expect(
      within(container)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual([telUri(DIALS), telUri(DIALS)]);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
