/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentClientsCard } from "./agent-clients";
import {
  AGENT_CLIENT_SCOPES,
  agentClientList,
  GEMINI_CLIENT,
} from "./agent-clients.fixtures";
import {
  json,
  MCP_URL,
  renderWithClient,
  serve,
  validationProblem,
} from "./agent-clients.testkit";

const realClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (realClipboard) {
    Object.defineProperty(navigator, "clipboard", realClipboard);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
});

const GEMINI_REDIRECT =
  "https://vertexaisearch.cloud.google.com/oauth-redirect";
const SECRET = "mcs_once_only_secret";

type User = ReturnType<typeof userEvent.setup>;

async function openDialog(user: User) {
  await user.click(
    await screen.findByRole("button", { name: "Register a client" }),
  );
  return screen.findByRole("dialog");
}

async function fill(
  user: User,
  dialog: HTMLElement,
  answers: Readonly<{ name: string; redirect: string }>,
) {
  const form = within(dialog);
  await user.type(form.getByRole("textbox", { name: /Name/ }), answers.name);
  await user.type(
    form.getByRole("combobox", { name: /Redirect URIs/ }),
    `${answers.redirect}{Enter}`,
  );
}

function registering(
  answer: (body: unknown) => Response | Promise<Response> = () =>
    json(
      {
        client: GEMINI_CLIENT,
        client_secret: SECRET,
        scopes: AGENT_CLIENT_SCOPES,
      },
      201,
    ),
) {
  return serve({
    "GET /installation/agent-clients": () => json(agentClientList([])),
    "POST /installation/agent-clients": (call) => answer(call.body),
  });
}

describe("Register a client", () => {
  it("sends the answers and shows what to paste, the secret only once", async () => {
    const user = userEvent.setup();
    const calls = registering();
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Gemini Enterprise",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    expect(await screen.findByText("Client registered")).toBeTruthy();
    expect(
      calls.find((call) => call.key === "POST /installation/agent-clients")
        ?.body,
    ).toEqual({
      name: "Gemini Enterprise",
      redirect_uris: [GEMINI_REDIRECT],
    });
    const value = (name: string) =>
      screen.getByRole("textbox", { name }).textContent;
    expect(value("Client ID")).toBe(GEMINI_CLIENT.client_id);
    expect(value("Client secret")).toBe(SECRET);
    expect(value("Authorization URL")).toBe(
      "https://crm.acme.test/oauth/authorize",
    );
    expect(value("Token URL, also for refresh")).toBe(
      "https://crm.acme.test/oauth/token",
    );
    expect(value("Scopes")).toBe("read draft write send enrich offline_access");
    expect(value("MCP server URL")).toBe(MCP_URL);
    expect(screen.getByText("Copy the secret now")).toBeTruthy();
    expect(screen.getByText("In Gemini Enterprise")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByText(SECRET)).toBeNull());
    const again = await openDialog(user);
    expect(within(again).getByText("Register a client")).toBeTruthy();
    expect(
      (within(again).getByRole("textbox", { name: /Name/ }) as HTMLInputElement)
        .value,
    ).toBe("");
    expect(screen.queryByText(SECRET)).toBeNull();
  });

  it("puts a refused redirect URI on its own field, in words", async () => {
    const user = userEvent.setup();
    registering(() =>
      validationProblem("redirect_uris", "invalid_redirect_uri"),
    );
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Tool",
      redirect: "http://evil.test/cb",
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    const message = await within(dialog).findByText(
      "Use https, or http on localhost or 127.0.0.1, with no fragment.",
    );
    expect(message.closest(".field")?.textContent).toContain("Redirect URIs");
    expect(
      within(dialog)
        .getByRole("combobox", { name: /Redirect URIs/ })
        .getAttribute("aria-invalid"),
    ).toBe("true");

    await user.type(within(dialog).getByRole("textbox", { name: /Name/ }), "s");
    expect(
      within(dialog).queryByText(
        "Use https, or http on localhost or 127.0.0.1, with no fragment.",
      ),
    ).toBeNull();
    expect(
      within(dialog)
        .getByRole("combobox", { name: /Redirect URIs/ })
        .getAttribute("aria-invalid"),
    ).not.toBe("true");
  });

  it("sends one registration while one is out, whatever submits the form", async () => {
    const user = userEvent.setup();
    let release: (answer: Response) => void = () => undefined;
    const calls = registering(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Tool",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    const form = dialog.querySelector("form");
    if (form === null) {
      throw new Error("the dialog has no form");
    }
    fireEvent.submit(form);
    expect(calls.filter((call) => call.key.startsWith("POST"))).toHaveLength(1);
    release(
      json(
        {
          client: GEMINI_CLIENT,
          client_secret: SECRET,
          scopes: AGENT_CLIENT_SCOPES,
        },
        201,
      ),
    );
    expect(await screen.findByText("Client registered")).toBeTruthy();
  });

  it("puts a refused name on the name field", async () => {
    const user = userEvent.setup();
    registering(() => validationProblem("name", "invalid_name"));
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Tool",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    expect(
      await within(dialog).findByText("Use a name of 1 to 120 characters."),
    ).toBeTruthy();
  });

  it("keeps a refusal that names no field under the form", async () => {
    const user = userEvent.setup();
    registering(() =>
      json({ title: "Forbidden", status: 403, detail: "Not yours." }, 403),
    );
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Tool",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    expect(await within(dialog).findByRole("alert")).toBeTruthy();
    expect(screen.queryByText("Client registered")).toBeNull();
  });

  it("refuses to send until the name and a redirect URI are given, and says which", async () => {
    const user = userEvent.setup();
    const calls = registering();
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    const form = within(dialog);
    expect(form.getByText("Give the client a name.")).toBeTruthy();
    await user.type(form.getByRole("textbox", { name: /Name/ }), "Tool");
    expect(form.getByText("Add at least one redirect URI.")).toBeTruthy();
    await user.click(form.getByRole("button", { name: /Register client/ }));
    expect(calls.some((call) => call.key.startsWith("POST"))).toBe(false);
    await user.type(
      form.getByRole("combobox", { name: /Redirect URIs/ }),
      `${GEMINI_REDIRECT}{Enter}`,
    );
    await user.click(form.getByRole("button", { name: /Register client/ }));
    expect(await screen.findByText("Client registered")).toBeTruthy();
  });

  // Codex sends no secret, so it connects on its own and is not offered here.
  it("lists the tools' callbacks as soon as the empty field is focused", async () => {
    const user = userEvent.setup();
    registering();
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await user.click(
      within(dialog).getByRole("combobox", { name: /Redirect URIs/ }),
    );
    const list = await screen.findByRole("listbox");
    for (const tool of [
      "Claude",
      "Claude Code",
      "ChatGPT",
      "Gemini Enterprise",
    ]) {
      expect(within(list).getByText(tool)).toBeTruthy();
    }
    expect(within(list).queryByText("Codex")).toBeNull();
    await user.click(within(list).getByText("Claude Code"));
    expect(within(dialog).getByText("http://localhost/callback")).toBeTruthy();
  });

  it("leaves the addresses out while the connector is off, and says so", async () => {
    const user = userEvent.setup();
    serve({
      "GET /installation/agent-clients": () => json(agentClientList([])),
      "POST /installation/agent-clients": () =>
        json(
          {
            client: GEMINI_CLIENT,
            client_secret: SECRET,
            scopes: AGENT_CLIENT_SCOPES,
          },
          201,
        ),
      "GET /.well-known/oauth-protected-resource": () =>
        new Response("{}", { status: 404 }),
    });
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Gemini Enterprise",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    expect(
      await screen.findByText("AI tools cannot connect to Margince yet."),
    ).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Client secret" })).toBeTruthy();
    expect(
      screen.queryByRole("textbox", { name: "Authorization URL" }),
    ).toBeNull();
  });

  it("says Copied only on the value the clipboard now holds", async () => {
    const user = userEvent.setup();
    registering();
    renderWithClient(<AgentClientsCard />);
    const dialog = await openDialog(user);
    await fill(user, dialog, {
      name: "Gemini Enterprise",
      redirect: GEMINI_REDIRECT,
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Register client" }),
    );
    await screen.findByText("Client registered");
    const writeText = vi.fn(async () => undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    await user.click(
      screen.getByRole("button", { name: "Copy Client secret" }),
    );
    expect(writeText).toHaveBeenCalledWith(SECRET);
    expect(
      await screen.findByRole("button", { name: "Copied Client secret" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Copy Client ID" }));
    expect(
      await screen.findByRole("button", { name: "Copied Client ID" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Copy Client secret" }),
    ).toBeTruthy();
  });
});
