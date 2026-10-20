/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentClientsCard } from "./agent-clients";
import {
  AGENT_CLIENT_SCOPES,
  agentClientList,
  GEMINI_CLIENT,
  PUBLISHED_CLIENT,
  SELF_REGISTERED_CLIENT,
} from "./agent-clients.fixtures";
import { json, renderWithClient, serve } from "./agent-clients.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const EVERY_CLIENT = [GEMINI_CLIENT, SELF_REGISTERED_CLIENT, PUBLISHED_CLIENT];

function row(client: { id: string }): HTMLElement {
  return screen.getByTestId(`agent-client-${client.id}`);
}

async function openMenu(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Actions for ${name}` }),
  );
}

describe("MCP clients", () => {
  it("lists each client with how it registered, its users and its state", async () => {
    serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList(EVERY_CLIENT)),
    });
    renderWithClient(<AgentClientsCard />);
    await screen.findByText("Gemini Enterprise");
    const gemini = within(row(GEMINI_CLIENT));
    expect(gemini.getByText("mc_7f3a9d2c41e8")).toBeTruthy();
    expect(gemini.getByText("By an admin · secret")).toBeTruthy();
    expect(gemini.getByText("12")).toBeTruthy();
    expect(gemini.queryByText("Disabled")).toBeNull();
    const old = within(row(SELF_REGISTERED_CLIENT));
    expect(old.getByText("Registered itself")).toBeTruthy();
    expect(old.getByText("Never")).toBeTruthy();
    expect(old.getByText("Disabled")).toBeTruthy();
    expect(
      within(row(PUBLISHED_CLIENT)).getByText("Published identity"),
    ).toBeTruthy();
  });

  it("says so when there is no client yet", async () => {
    serve({
      "GET /installation/agent-clients": () => json(agentClientList([])),
    });
    renderWithClient(<AgentClientsCard />);
    expect(await screen.findByText("No MCP client yet.")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Register a client" }),
    ).toBeTruthy();
  });

  it("draws nothing and asks nothing for a reader without the grant", async () => {
    const calls = serve(
      { "GET /installation/agent-clients": () => json(agentClientList([])) },
      {},
    );
    const view = renderWithClient(<AgentClientsCard />);
    await waitFor(() =>
      expect(calls.some((call) => call.key === "GET /me")).toBe(true),
    );
    expect(view.container.innerHTML).toBe("");
    expect(
      calls.some((call) => call.key === "GET /installation/agent-clients"),
    ).toBe(false);
  });

  it("shows a reader who may only look no verbs, and says who may change them", async () => {
    serve(
      {
        "GET /installation/agent-clients": () =>
          json(agentClientList(EVERY_CLIENT)),
      },
      { agent_client: ["read"] },
    );
    renderWithClient(<AgentClientsCard />);
    expect(
      await screen.findByText("Only an admin can change MCP clients."),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Register a client" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Actions for Gemini Enterprise" }),
    ).toBeNull();
  });

  it("disables and enables a client at once, without asking", async () => {
    const user = userEvent.setup();
    const calls = serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList(EVERY_CLIENT)),
      [`PATCH /installation/agent-clients/${GEMINI_CLIENT.id}`]: () =>
        json({ ...GEMINI_CLIENT, disabled: true }),
      [`PATCH /installation/agent-clients/${SELF_REGISTERED_CLIENT.id}`]: () =>
        json({ ...SELF_REGISTERED_CLIENT, disabled: false }),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Gemini Enterprise");
    await user.click(screen.getByRole("button", { name: "Disable" }));
    await openMenu(user, "Old desktop tool");
    await user.click(screen.getByRole("button", { name: "Enable" }));
    await waitFor(() =>
      expect(
        calls
          .filter((call) => call.key.startsWith("PATCH"))
          .map((call) => [call.key, call.body]),
      ).toEqual([
        [
          `PATCH /installation/agent-clients/${GEMINI_CLIENT.id}`,
          { disabled: true },
        ],
        [
          `PATCH /installation/agent-clients/${SELF_REGISTERED_CLIENT.id}`,
          { disabled: false },
        ],
      ]),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says when a disable was refused", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList([GEMINI_CLIENT])),
      [`PATCH /installation/agent-clients/${GEMINI_CLIENT.id}`]: () =>
        json({ title: "Not Found", status: 404, detail: "Gone." }, 404),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Gemini Enterprise");
    await user.click(screen.getByRole("button", { name: "Disable" }));
    expect(await screen.findByText("The client was not changed")).toBeTruthy();
  });

  it("offers a rotate only for an admin's client with a secret", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList(EVERY_CLIENT)),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Claude Code");
    expect(screen.queryByRole("button", { name: "Rotate secret…" })).toBeNull();
    await openMenu(user, "Gemini Enterprise");
    expect(screen.getByRole("button", { name: "Rotate secret…" })).toBeTruthy();
  });

  it("tells an admin deleting a DCR client that the switch keeps such tools out", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList(EVERY_CLIENT)),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Old desktop tool");
    await user.click(screen.getByRole("button", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        "Every connection through Old desktop tool ends. The tool can register itself again. To keep tools that register themselves out, turn off “Let tools register themselves”.",
      ),
    ).toBeTruthy();
  });

  it("deletes a published-identity client after saying it comes back", async () => {
    const user = userEvent.setup();
    let listed = EVERY_CLIENT;
    const calls = serve({
      "GET /installation/agent-clients": () => json(agentClientList(listed)),
      [`DELETE /installation/agent-clients/${PUBLISHED_CLIENT.id}`]: () => {
        listed = [GEMINI_CLIENT, SELF_REGISTERED_CLIENT];
        return new Response(null, { status: 204 });
      },
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Claude Code");
    await user.click(screen.getByRole("button", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Delete Claude Code?")).toBeTruthy();
    expect(
      within(dialog).getByText(
        "Every connection through Claude Code ends. The tool comes back the next time someone connects it. To keep it out, disable it instead.",
      ),
    ).toBeTruthy();
    await user.click(
      within(dialog).getByRole("button", { name: "Delete client" }),
    );
    await waitFor(() => expect(screen.queryByText("Claude Code")).toBeNull());
    expect(
      calls.filter((call) => call.key.startsWith("DELETE")).map((c) => c.key),
    ).toEqual([`DELETE /installation/agent-clients/${PUBLISHED_CLIENT.id}`]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says an admin's client is gone for good", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList([GEMINI_CLIENT])),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Gemini Enterprise");
    await user.click(screen.getByRole("button", { name: "Delete…" }));
    expect(
      await screen.findByText(
        "Every connection through Gemini Enterprise ends. The client is gone for good.",
      ),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("rotates a secret and shows the new one once", async () => {
    const user = userEvent.setup();
    const calls = serve({
      "GET /installation/agent-clients": () =>
        json(agentClientList([GEMINI_CLIENT])),
      [`POST /installation/agent-clients/${GEMINI_CLIENT.id}/secret`]: () =>
        json({
          client_secret: "mcs_rotated_secret_value",
          scopes: AGENT_CLIENT_SCOPES,
        }),
    });
    renderWithClient(<AgentClientsCard />);
    await openMenu(user, "Gemini Enterprise");
    await user.click(screen.getByRole("button", { name: "Rotate secret…" }));
    expect(
      await screen.findByText("Rotate the secret of Gemini Enterprise?"),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Rotate secret" }));
    expect(await screen.findByText("New client secret")).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "Client secret" }).textContent,
    ).toBe("mcs_rotated_secret_value");
    expect(screen.getByRole("textbox", { name: "Client ID" }).textContent).toBe(
      "mc_7f3a9d2c41e8",
    );
    expect(screen.getByText("Copy the secret now")).toBeTruthy();
    expect(
      calls.filter((call) => call.key.startsWith("POST")).map((c) => c.key),
    ).toEqual([`POST /installation/agent-clients/${GEMINI_CLIENT.id}/secret`]);
    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(screen.queryByText("mcs_rotated_secret_value")).toBeNull(),
    );
    await openMenu(user, "Gemini Enterprise");
    await user.click(screen.getByRole("button", { name: "Rotate secret…" }));
    expect(
      await screen.findByText("Rotate the secret of Gemini Enterprise?"),
    ).toBeTruthy();
    expect(screen.queryByText("mcs_rotated_secret_value")).toBeNull();
  });

  it("reads the next page when asked, after the rows already shown", async () => {
    const user = userEvent.setup();
    const calls = serve({
      "GET /installation/agent-clients": (call) =>
        call.search.includes("cursor=page-2")
          ? json(agentClientList([PUBLISHED_CLIENT]))
          : json(agentClientList([GEMINI_CLIENT], undefined, "page-2")),
    });
    renderWithClient(<AgentClientsCard />);
    await screen.findByText("Gemini Enterprise");
    expect(screen.queryByText("Claude Code")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Claude Code")).toBeTruthy();
    expect(screen.getByText("Gemini Enterprise")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
    expect(
      calls
        .filter((call) => call.key === "GET /installation/agent-clients")
        .map((call) => call.search),
    ).toEqual(["", "?cursor=page-2"]);
  });
});
