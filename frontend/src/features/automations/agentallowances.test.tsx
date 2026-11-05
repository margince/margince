/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider, translate } from "../../i18n";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AgentAllowancesCard } from "./agentallowances";

// Settings → AI usage: what one passport may do per window. The values read at
// rest; an admin opens a form, changes what moved, and saves them in one
// request. A value past its bounds is refused in the box, and a saved window
// length is reported as waiting for its reset.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const EDITOR: GrantSpec = { installation_settings: ["read", "update"] };
const READER: GrantSpec = { installation_settings: ["read"] };

function backendFor(
  allow: GrantSpec,
  allowances: object = defaultAgentAllowances,
  locks: unknown[] = [],
) {
  let agent_allowances = { ...allowances };
  const patches: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (url.pathname.endsWith("/installation/settings")) {
        if (req.method === "PATCH") {
          const patch = await req.json();
          patches.push(patch);
          agent_allowances = { ...agent_allowances, ...(patch as object) };
        }
        return jsonResponse({ agent_allowances, locks });
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  return { fetchMock, patches };
}

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function openForm(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", {
      name: translate("en", "agentAllowances.edit"),
    }),
  );
}

describe("AgentAllowancesCard", () => {
  it("shows the three allowances at rest, with no inputs", async () => {
    vi.stubGlobal("fetch", backendFor(EDITOR).fetchMock);
    render(<AgentAllowancesCard />);

    const readout = await screen.findByTestId("allowance-readout");
    expect(readout).toHaveTextContent("Changes");
    expect(readout).toHaveTextContent("200");
    expect(readout).toHaveTextContent("Sends");
    expect(readout).toHaveTextContent("20");
    expect(readout).toHaveTextContent("Calls in total");
    expect(readout).toHaveTextContent("1,000");
    expect(readout).not.toHaveTextContent("Records read");
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(
      screen.getByText(translate("en", "agentAllowances.state.defaults")),
    ).toBeTruthy();
    expect(screen.getByText(/Window: 1 day/)).toBeTruthy();
  });

  it("says so when a value is off its default", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, {
        ...defaultAgentAllowances,
        agent_calls_per_window: 60,
      }).fetchMock,
    );
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByText(translate("en", "agentAllowances.state.changed")),
    ).toBeTruthy();
  });

  it.each([
    [
      "agent_writes_per_window",
      "10",
      "5001",
      "Enter a whole number from 10 to 5,000.",
    ],
    [
      "agent_egress_per_window",
      "5",
      "0",
      "Enter a whole number from 1 to 200.",
    ],
    [
      "agent_calls_per_window",
      "50",
      "49",
      "Enter a whole number from 50 to 20,000.",
    ],
  ] as const)(
    "saves %s alone and refuses past its bounds",
    async (property, ok, bad, refusal) => {
      const backend = backendFor(EDITOR);
      vi.stubGlobal("fetch", backend.fetchMock);
      const user = userEvent.setup();
      render(<AgentAllowancesCard />);
      await openForm(user);

      const box = await screen.findByTestId<HTMLInputElement>(
        `allowance-${property}`,
      );
      expect(box.value).toBe(String(defaultAgentAllowances[property]));
      await user.clear(box);
      await user.type(box, bad);
      expect(await screen.findByText(refusal)).toBeTruthy();
      expect(box).toHaveAccessibleDescription(expect.stringContaining(refusal));
      const save = screen.getByRole("button", {
        name: translate("en", "agentAllowances.save"),
      });
      expect(save).toBeDisabled();
      expect(backend.patches).toEqual([]);

      await user.clear(box);
      await user.type(box, ok);
      await user.click(save);
      await waitFor(() =>
        expect(backend.patches).toEqual([{ [property]: Number(ok) }]),
      );
      await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
    },
  );

  it("sends one request for every value that moved, and nothing for the rest", async () => {
    const backend = backendFor(EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);
    await openForm(user);

    const writes = await screen.findByTestId<HTMLInputElement>(
      "allowance-agent_writes_per_window",
    );
    const calls = screen.getByTestId<HTMLInputElement>(
      "allowance-agent_calls_per_window",
    );
    await user.clear(writes);
    await user.type(writes, "100");
    await user.clear(calls);
    await user.type(calls, "500");

    expect(screen.getByText(/Changes 200 → 100/)).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: translate("en", "agentAllowances.save"),
      }),
    );
    await waitFor(() =>
      expect(backend.patches).toEqual([
        { agent_writes_per_window: 100, agent_calls_per_window: 500 },
      ]),
    );
  });

  it("lets the window length be picked, and says it starts at the next reset", async () => {
    const backend = backendFor(EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);
    await openForm(user);

    await pickOption(
      user,
      await screen.findByTestId("allowance-agent_window_hours"),
      "12 hours",
    );

    expect(screen.getByText(/Window 1 day → 12 hours/)).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: translate("en", "agentAllowances.save"),
      }),
    );
    await waitFor(() =>
      expect(backend.patches).toEqual([{ agent_window_hours: 12 }]),
    );
  });

  it("keeps a window length that is not one of the choices", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, { ...defaultAgentAllowances, agent_window_hours: 5 })
        .fetchMock,
    );
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);
    await openForm(user);

    expect(
      await screen.findByTestId("allowance-agent_window_hours"),
    ).toHaveTextContent("5 hours");
  });

  it("says when a saved window length takes over, and shows it in the form", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, {
        ...defaultAgentAllowances,
        agent_window_next_hours: 12,
        agent_window_next_starts_at: "2026-10-15T00:00:00Z",
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByText(translate("en", "agentAllowances.pending.title")),
    ).toBeTruthy();
    expect(screen.getByText(/Each window lasts 12 hours/)).toBeTruthy();
    await openForm(user);
    expect(
      screen.getByTestId("allowance-agent_window_hours"),
    ).toHaveTextContent("12 hours");
  });

  it("fills the defaults from the server and cancels without saving", async () => {
    const backend = backendFor(EDITOR, {
      ...defaultAgentAllowances,
      agent_calls_per_window: 60,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);
    await openForm(user);

    await user.click(
      await screen.findByRole("button", {
        name: translate("en", "agentAllowances.useDefaults"),
      }),
    );
    expect(
      screen.getByTestId<HTMLInputElement>("allowance-agent_calls_per_window")
        .value,
    ).toBe("1000");
    await user.click(
      screen.getByRole("button", {
        name: translate("en", "agentAllowances.cancel"),
      }),
    );

    expect(screen.queryByRole("textbox")).toBeNull();
    expect(backend.patches).toEqual([]);
  });

  it("shows the fixed MCP limits and when the window resets", async () => {
    vi.stubGlobal("fetch", backendFor(EDITOR).fetchMock);
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByText(/240 calls and 30 stream opens/),
    ).toBeTruthy();
    expect(
      screen.getByText(/Every allowance resets .*2026.* UTC/),
    ).toBeTruthy();
  });

  it("refuses Edit, with the reason, when a boot source sets every allowance", async () => {
    const reason =
      "Set by agent.allowances in /etc/margince/margince.yaml. Change it there.";
    const locks = [
      "agent_writes_per_window",
      "agent_egress_per_window",
      "agent_calls_per_window",
      "agent_window_hours",
    ].map((property) => ({
      key: `installation.${property}`,
      origin: {
        source: "config_file",
        name: "agent.allowances",
        file: "/etc/margince/margince.yaml",
      },
    }));
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, defaultAgentAllowances, locks).fetchMock,
    );
    render(<AgentAllowancesCard />);

    const edit = await screen.findByRole("button", {
      name: translate("en", "agentAllowances.edit"),
    });
    expect(edit).toBeDisabled();
    expect(screen.getAllByText(reason)).toHaveLength(1);
  });

  it("keeps Edit open while any allowance is the administrator's", async () => {
    const locks = [
      {
        key: "installation.agent_window_hours",
        origin: {
          source: "env",
          name: "MARGINCE_INSTALLATION_AGENT_WINDOW_HOURS",
        },
      },
    ];
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, defaultAgentAllowances, locks).fetchMock,
    );
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByRole("button", {
        name: translate("en", "agentAllowances.edit"),
      }),
    ).toBeEnabled();
  });

  it("offers no Edit button to a seat without the update grant", async () => {
    vi.stubGlobal("fetch", backendFor(READER).fetchMock);
    render(<AgentAllowancesCard />);

    expect(await screen.findByTestId("allowance-readout")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: translate("en", "agentAllowances.edit"),
      }),
    ).toBeNull();
    expect(
      screen.getByText(translate("en", "operations.adminOnly")),
    ).toBeTruthy();
  });

  it("draws nothing for a seat that cannot read the settings", async () => {
    vi.stubGlobal("fetch", backendFor({}).fetchMock);
    const { container } = render(<AgentAllowancesCard />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

it("redraws the usage bars against a saved allowance", async () => {
  const backend = backendFor(EDITOR);
  vi.stubGlobal("fetch", backend.fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const user = userEvent.setup();
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <AgentAllowancesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  await openForm(user);
  const box = await screen.findByTestId<HTMLInputElement>(
    "allowance-agent_writes_per_window",
  );
  await user.clear(box);
  await user.type(box, "50");
  await user.click(
    screen.getByRole("button", {
      name: translate("en", "agentAllowances.save"),
    }),
  );

  await waitFor(() =>
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["agent-usage"] }),
  );
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["passports"] });
});
