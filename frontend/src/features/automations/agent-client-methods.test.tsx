/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentClientMethodsCard } from "./agent-client-methods";
import { agentClientList, BOTH_METHODS_ON } from "./agent-clients.fixtures";
import { json, renderWithClient, serve } from "./agent-clients.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const PUBLISHED = "Accept a tool’s published identity";
const DYNAMIC = "Let tools register themselves";

function switchNamed(name: string) {
  const control = screen.getByRole("switch", { name });
  return {
    checked: control.getAttribute("aria-checked") === "true",
    disabled: control.hasAttribute("disabled"),
  };
}

describe("How MCP clients may connect", () => {
  it("shows each switch at the admin's setting, and the admin's own way as always on", async () => {
    serve({
      "GET /installation/agent-clients": () =>
        json(
          agentClientList([], {
            ...BOTH_METHODS_ON,
            dynamic: {
              enabled: false,
              setting: false,
              deployment_allows: true,
            },
          }),
        ),
    });
    renderWithClient(<AgentClientMethodsCard />);
    await screen.findByRole("switch", { name: PUBLISHED });
    expect(switchNamed(PUBLISHED).checked).toBe(true);
    expect(switchNamed(DYNAMIC).checked).toBe(false);
    expect(switchNamed(DYNAMIC).disabled).toBe(false);
    expect(screen.getByText("Clients an admin registers")).toBeTruthy();
    expect(screen.getByText("Always on")).toBeTruthy();
    expect(screen.queryByText("Only an admin can change these.")).toBeNull();
  });

  it("links each automatic way to its section of the MCP specification", async () => {
    serve({
      "GET /installation/agent-clients": () => json(agentClientList([])),
    });
    renderWithClient(<AgentClientMethodsCard />);
    const cimd = await screen.findByRole("link", { name: /\(CIMD\)/ });
    const dcr = screen.getByRole("link", { name: /\(DCR\)/ });
    expect(cimd.getAttribute("href")).toBe(
      "https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization#client-id-metadata-documents",
    );
    expect(dcr.getAttribute("href")).toBe(
      "https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization#dynamic-client-registration",
    );
    expect(cimd.getAttribute("target")).toBe("_blank");
  });

  it("keeps the switch focusable while its save is out", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () => json(agentClientList([])),
      "PATCH /installation/settings": () =>
        new Promise<Response>(() => undefined),
    });
    renderWithClient(<AgentClientMethodsCard />);
    const control = await screen.findByRole("switch", { name: DYNAMIC });
    control.focus();
    await user.keyboard(" ");
    await waitFor(() =>
      expect(control.getAttribute("aria-disabled")).toBe("true"),
    );
    expect(switchNamed(DYNAMIC).disabled).toBe(false);
    expect(document.activeElement).toBe(control);
  });

  it("writes the switch's own settings field, and reads the list again", async () => {
    const user = userEvent.setup();
    let dynamic = false;
    const calls = serve({
      "GET /installation/agent-clients": () =>
        json(
          agentClientList([], {
            ...BOTH_METHODS_ON,
            dynamic: {
              enabled: dynamic,
              setting: dynamic,
              deployment_allows: true,
            },
          }),
        ),
      "PATCH /installation/settings": (call) => {
        dynamic = (call.body as Record<string, boolean>)
          .dynamic_client_registration;
        return json({});
      },
    });
    renderWithClient(<AgentClientMethodsCard />);
    await user.click(await screen.findByRole("switch", { name: DYNAMIC }));
    await waitFor(() => expect(switchNamed(DYNAMIC).checked).toBe(true));
    expect(
      calls.filter((call) => call.key === "PATCH /installation/settings"),
    ).toEqual([
      {
        key: "PATCH /installation/settings",
        search: "",
        body: { dynamic_client_registration: true },
      },
    ]);
    await user.click(screen.getByRole("switch", { name: PUBLISHED }));
    await waitFor(() =>
      expect(
        calls
          .filter((call) => call.key === "PATCH /installation/settings")
          .map((call) => call.body),
      ).toEqual([
        { dynamic_client_registration: true },
        { metadata_document_clients: false },
      ]),
    );
  });

  it("holds a way the deployment file turned off: off, locked, and saying why", async () => {
    serve({
      "GET /installation/agent-clients": () =>
        json(
          agentClientList([], {
            ...BOTH_METHODS_ON,
            dynamic: {
              enabled: false,
              setting: true,
              deployment_allows: false,
            },
          }),
        ),
    });
    renderWithClient(<AgentClientMethodsCard />);
    await screen.findByRole("switch", { name: DYNAMIC });
    expect(switchNamed(DYNAMIC).checked).toBe(false);
    expect(switchNamed(DYNAMIC).disabled).toBe(true);
    expect(switchNamed(PUBLISHED).disabled).toBe(false);
    expect(
      screen.getByText(
        /grows over time\. Whoever runs this installation turned this off in its configuration\./,
      ),
    ).toBeTruthy();
  });

  it("locks both switches for a reader who may not change them", async () => {
    serve(
      {
        "GET /installation/agent-clients": () => json(agentClientList([])),
      },
      { agent_client: ["read"] },
    );
    renderWithClient(<AgentClientMethodsCard />);
    await screen.findByRole("switch", { name: PUBLISHED });
    expect(switchNamed(PUBLISHED).disabled).toBe(true);
    expect(switchNamed(DYNAMIC).disabled).toBe(true);
    expect(screen.getByText("Only an admin can change these.")).toBeTruthy();
  });

  it("says when a save was refused", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () => json(agentClientList([])),
      "PATCH /installation/settings": () =>
        json({ title: "Unavailable", status: 503, detail: "Try later." }, 503),
    });
    renderWithClient(<AgentClientMethodsCard />);
    await user.click(await screen.findByRole("switch", { name: DYNAMIC }));
    expect(await screen.findByText("The change was not saved")).toBeTruthy();
  });

  it("draws nothing and asks nothing for a reader without the grant", async () => {
    const calls = serve(
      { "GET /installation/agent-clients": () => json(agentClientList([])) },
      { installation_settings: ["read"] },
    );
    const view = renderWithClient(<AgentClientMethodsCard />);
    await waitFor(() =>
      expect(calls.some((call) => call.key === "GET /me")).toBe(true),
    );
    expect(view.container.innerHTML).toBe("");
    expect(
      calls.some((call) => call.key === "GET /installation/agent-clients"),
    ).toBe(false);
  });
});
