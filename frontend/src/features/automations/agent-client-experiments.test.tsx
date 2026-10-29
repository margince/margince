/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
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

const CHECK = "Check what an AI tool supports";

function settings(on: boolean) {
  return { mcp_client_check: on };
}

describe("Experiments", () => {
  it("shows nothing to a seat that may not change MCP clients", async () => {
    serve({}, { agent_client: ["read"] });
    renderWithClient(<AgentClientExperimentsCard />);
    await waitFor(() => expect(screen.queryByText("Experiments")).toBeNull());
  });

  it("shows the check off, and turns it on through the settings patch", async () => {
    const user = userEvent.setup();
    let on = false;
    const calls = serve(
      {
        "GET /installation/settings": () => json(settings(on)),
        "PATCH /installation/settings": ({ body }) => {
          on = (body as { mcp_client_check: boolean }).mcp_client_check;
          return json(settings(on));
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

  it("opens the explanation with the question to ask, and Escape closes it", async () => {
    const user = userEvent.setup();
    serve(
      { "GET /installation/settings": () => json(settings(false)) },
      ADMIN_GRANTS,
    );
    renderWithClient(<AgentClientExperimentsCard />);
    const trigger = await screen.findByRole("button", {
      name: "About this experiment",
    });
    await user.click(trigger);
    expect(await screen.findByText("How to test")).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "The question to ask" }).textContent,
    ).toContain("check_client");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByText("How to test")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });
});
