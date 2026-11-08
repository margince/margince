/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useCan } from "../../lib/capability";
import { agentClientList } from "../../testing/agent-clients.fixtures";
import { AgentClientExperimentsCard } from "./agent-client-experiments";
import {
  ADMIN_GRANTS,
  json,
  renderWithClient,
  serve,
} from "./agent-clients.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CHECK = "Tool support check";

// Renders once the seat's grants have loaded, so an absent card is measured
// against the seat, not against a /me still in flight.
function GrantsLoaded() {
  return useCan("deal", "read") ? <p>grants loaded</p> : null;
}

function clientList(on: boolean) {
  return { ...agentClientList([]), client_check: on };
}

describe("Experiments", () => {
  it("shows nothing to a seat that may not read MCP clients", async () => {
    serve({}, { deal: ["read"] });
    renderWithClient(
      <>
        <GrantsLoaded />
        <AgentClientExperimentsCard />
      </>,
    );
    await screen.findByText("grants loaded");
    expect(screen.queryByText("Experiments")).toBeNull();
  });

  it("shows a reader the switch, but not to change", async () => {
    serve(
      { "GET /installation/agent-clients": () => json(clientList(true)) },
      { agent_client: ["read"] },
    );
    renderWithClient(<AgentClientExperimentsCard />);
    const control = await screen.findByRole("switch", { name: CHECK });
    expect(control.getAttribute("aria-checked")).toBe("true");
    expect((control as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Only an admin can change these.")).toBeTruthy();
  });

  it("says a refused change, and names it from the switch", async () => {
    const user = userEvent.setup();
    serve(
      {
        "GET /installation/agent-clients": () => json(clientList(false)),
        "PATCH /installation/settings": () =>
          json(
            { title: "Forbidden", status: 403, detail: "Not allowed." },
            403,
          ),
      },
      ADMIN_GRANTS,
    );
    renderWithClient(<AgentClientExperimentsCard />);
    await user.click(await screen.findByRole("switch", { name: CHECK }));
    const refusal = await screen.findByText("The change was not saved");
    const callout = refusal.closest("[id]");
    const describedBy = screen
      .getByRole("switch", { name: CHECK })
      .getAttribute("aria-describedby");
    expect(callout?.id).toBeTruthy();
    expect(describedBy?.split(" ")).toContain(callout?.id);
  });

  it("shows the check off, and turns it on through the settings patch", async () => {
    const user = userEvent.setup();
    let on = false;
    const calls = serve(
      {
        "GET /installation/agent-clients": () => json(clientList(on)),
        "PATCH /installation/settings": ({ body }) => {
          on = (body as { mcp_client_check: boolean }).mcp_client_check;
          return json({});
        },
      },
      ADMIN_GRANTS,
    );
    renderWithClient(<AgentClientExperimentsCard />);
    const control = await screen.findByRole("switch", { name: CHECK });
    expect(control.getAttribute("aria-checked")).toBe("false");
    await user.click(control);
    await waitFor(() =>
      expect(
        screen
          .getByRole("switch", { name: CHECK })
          .getAttribute("aria-checked"),
      ).toBe("true"),
    );
    expect(
      calls.find((c) => c.key === "PATCH /installation/settings")?.body,
    ).toEqual({ mcp_client_check: true });
  });

  it("says the switch could not be read when the client list fails", async () => {
    serve(
      {
        "GET /installation/agent-clients": () =>
          json({ title: "Unavailable", status: 503 }, 503),
      },
      ADMIN_GRANTS,
    );
    renderWithClient(<AgentClientExperimentsCard />);
    expect(
      await screen.findByText("Could not load this view. Reload the page."),
    ).toBeTruthy();
  });

  it("opens the explanation with the question to ask, and Escape closes it", async () => {
    const user = userEvent.setup();
    serve(
      { "GET /installation/agent-clients": () => json(clientList(false)) },
      ADMIN_GRANTS,
    );
    renderWithClient(<AgentClientExperimentsCard />);
    const trigger = await screen.findByRole("button", {
      name: "About this experiment",
    });
    await user.click(trigger);
    expect(await screen.findByText("How to test")).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "Question" }).textContent,
    ).toContain("check_client");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByText("How to test")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });
});
