/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingList } from "../design-system/settingrow";
import { LocaleProvider } from "../i18n";
import type { MfaStatus } from "./mfa.queries";
import { MfaSettingRow } from "./mfacard";

const SECRET = "JBSWY3DPEHPK3PXP";
const CODES = ["aB3-dEf_ghIJkl", "Zx9_yW8-vU7tsr"];

const OFF: MfaStatus = {
  enrolled: false,
  confirmed: false,
  recovery_codes_left: 0,
};
const ON: MfaStatus = {
  enrolled: true,
  confirmed: true,
  recovery_codes_left: 10,
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function problem(status: number, code?: string) {
  return json({ title: "Refused", status, code }, status);
}

type Answer = (body: unknown) => Response | Promise<Response>;

// One route table per test: GET /me/mfa answers `status`, and each write
// answers from its queue, so a test states the server's replies in order.
function serve(
  initial: MfaStatus,
  writes: Partial<Record<string, Answer[]>> = {},
) {
  const server = { status: initial, statusReads: 0, sent: [] as unknown[] };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const route = `${request.method} ${new URL(request.url).pathname}`;
      if (route === "GET /v1/me/mfa") {
        server.statusReads += 1;
        return json(server.status);
      }
      const text = await request.text();
      const body: unknown = text === "" ? null : JSON.parse(text);
      server.sent.push({ route, body });
      const answer = writes[route]?.shift();
      return answer ? answer(body) : problem(500);
    }),
  );
  return server;
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SettingList>
          <MfaSettingRow />
        </SettingList>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

const ENROL: Answer = () =>
  json({
    secret: SECRET,
    otpauth_uri: `otpauth://totp/Margince:a%40b.test?secret=${SECRET}`,
  });

const codeField = () => screen.getByRole("textbox", { name: /code/i });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the two-step sign-in row", () => {
  it("sets up an authenticator, refuses a wrong code, then shows the recovery codes once", async () => {
    const user = userEvent.setup();
    const server = serve(OFF, {
      "POST /v1/me/mfa/totp": [ENROL],
      "POST /v1/me/mfa/totp/confirm": [
        () => problem(401),
        () => {
          server.status = ON;
          return json({ recovery_codes: CODES });
        },
      ],
    });
    const client = mount();

    await screen.findByText("Not set up");
    await user.click(screen.getByRole("button", { name: "Set up" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByRole("img", { name: /qr code/i }),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByRole("textbox", { name: "Secret key" }),
    ).toHaveTextContent(SECRET);

    await user.type(codeField(), "000 000");
    await user.click(within(dialog).getByRole("button", { name: "Turn on" }));
    expect(
      await within(dialog).findByText(/code was not accepted/i),
    ).toBeInTheDocument();
    expect(codeField()).toHaveValue("");

    await user.type(codeField(), "123 456");
    await user.click(within(dialog).getByRole("button", { name: "Turn on" }));
    const codes = await within(dialog).findByRole("textbox", {
      name: "Recovery codes",
    });
    expect(codes).toHaveTextContent(CODES[0]);
    expect(codes).toHaveTextContent(CODES[1]);
    expect(within(dialog).getByText(/not shown again/i)).toBeInTheDocument();
    expect(server.sent).toContainEqual({
      route: "POST /v1/me/mfa/totp/confirm",
      body: { code: "123456" },
    });

    const readsBefore = server.statusReads;
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    expect(
      await screen.findByText("On · 10 recovery codes left"),
    ).toBeInTheDocument();
    expect(server.statusReads).toBeGreaterThan(readsBefore);
    await waitFor(() => expect(client.getMutationCache().getAll()).toEqual([]));
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.state.data),
      ),
    ).not.toContain(SECRET);
  });

  it("says why setup cannot start when a factor is already on", async () => {
    const user = userEvent.setup();
    serve(OFF, {
      "POST /v1/me/mfa/totp": [() => problem(409, "mfa_already_enrolled")],
    });
    mount();

    await user.click(await screen.findByRole("button", { name: "Set up" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/already on/i)).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("counts the recovery codes left and warns when few remain", async () => {
    serve({ ...ON, recovery_codes_left: 1 });
    mount();

    expect(
      await screen.findByText("On · 1 recovery code left"),
    ).toBeInTheDocument();
    expect(screen.getByText("Few recovery codes left")).toBeInTheDocument();
    expect(
      screen.getByText(/remove two-step sign-in and set it up again/i),
    ).toBeInTheDocument();
  });

  it("does not warn while plenty of recovery codes remain", async () => {
    serve(ON);
    mount();

    await screen.findByText("On · 10 recovery codes left");
    expect(screen.queryByText("Few recovery codes left")).toBeNull();
    expect(screen.queryByRole("button", { name: "Set up" })).toBeNull();
  });

  it("removes the factor with a code the server checks, submitted with Enter", async () => {
    const user = userEvent.setup();
    const server = serve(ON, {
      "DELETE /v1/me/mfa": [
        () => {
          server.status = OFF;
          return new Response(null, { status: 204 });
        },
      ],
    });
    mount();

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    await screen.findByRole("dialog");
    await user.type(codeField(), " 654 321 {Enter}");

    expect(
      await screen.findByText("Two-step sign-in removed"),
    ).toBeInTheDocument();
    expect(server.sent).toEqual([
      { route: "DELETE /v1/me/mfa", body: { code: "654321" } },
    ]);
    expect(await screen.findByText("Not set up")).toBeInTheDocument();
  });

  it("shows a wrong code and a rate limit on removal, and keeps the factor", async () => {
    const user = userEvent.setup();
    serve(ON, {
      "DELETE /v1/me/mfa": [() => problem(401), () => problem(429)],
    });
    mount();

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", {
      name: "Remove two-step sign-in",
    });
    await user.type(codeField(), "111111");
    await user.click(confirm);
    expect(
      await within(dialog).findByText(/code was not accepted/i),
    ).toBeInTheDocument();
    expect(codeField()).toHaveValue("");

    await user.type(codeField(), "222222");
    await user.click(confirm);
    expect(
      await within(dialog).findByText(/too many wrong codes/i),
    ).toBeInTheDocument();
    expect(screen.getByText("On · 10 recovery codes left")).toBeInTheDocument();
  });

  it("discards an unfinished setup without asking for a code", async () => {
    const user = userEvent.setup();
    const server = serve(
      { enrolled: true, confirmed: false, recovery_codes_left: 0 },
      {
        "DELETE /v1/me/mfa": [
          () => {
            server.status = OFF;
            return new Response(null, { status: 204 });
          },
        ],
      },
    );
    mount();

    expect(await screen.findByText("Not finished")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Set up" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).queryByRole("textbox")).toBeNull();
    await user.click(
      within(dialog).getByRole("button", { name: "Discard setup" }),
    );

    expect(await screen.findByText("Setup discarded")).toBeInTheDocument();
    expect(server.sent).toEqual([
      { route: "DELETE /v1/me/mfa", body: { code: "" } },
    ]);
  });
  it("keeps the recovery codes on screen until Done", async () => {
    const user = userEvent.setup();
    serve(OFF, {
      "POST /v1/me/mfa/totp": [ENROL],
      "POST /v1/me/mfa/totp/confirm": [() => json({ recovery_codes: CODES })],
    });
    mount();

    await user.click(await screen.findByRole("button", { name: "Set up" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(
      await within(dialog).findByRole("textbox", {
        name: /code from the app/i,
      }),
      "123456{Enter}",
    );
    await within(dialog).findByRole("textbox", { name: "Recovery codes" });

    await user.keyboard("{Escape}");
    await user.click(within(dialog).getByRole("button", { name: /close/i }));
    const overlay = dialog.parentElement;
    if (overlay) await user.click(overlay);
    expect(screen.getByRole("dialog")).toHaveTextContent(CODES[0]);
    expect(
      within(dialog).getByText("Save the codes, then choose Done."),
    ).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("does not let a dismissal drop a confirm that is still on its way", async () => {
    const user = userEvent.setup();
    let answer: (response: Response) => void = () => undefined;
    serve(OFF, {
      "POST /v1/me/mfa/totp": [ENROL],
      "POST /v1/me/mfa/totp/confirm": [
        () =>
          new Promise<Response>((resolve) => {
            answer = resolve;
          }),
      ],
    });
    mount();

    await user.click(await screen.findByRole("button", { name: "Set up" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(
      await within(dialog).findByRole("textbox", {
        name: /code from the app/i,
      }),
      "123456{Enter}",
    );
    await user.keyboard("{Escape}");
    const overlay = dialog.parentElement;
    if (overlay) await user.click(overlay);
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    answer(json({ recovery_codes: CODES }));
    expect(
      await within(dialog).findByRole("textbox", { name: "Recovery codes" }),
    ).toHaveTextContent(CODES[1]);
  });

  it("says so when an unfinished setup was confirmed in another session", async () => {
    const user = userEvent.setup();
    const server = serve(
      { enrolled: true, confirmed: false, recovery_codes_left: 0 },
      {
        "DELETE /v1/me/mfa": [
          () => {
            server.status = ON;
            return problem(401);
          },
        ],
      },
    );
    mount();

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Discard setup" }),
    );

    expect(
      await within(dialog).findByText(/turned on in another session/i),
    ).toBeInTheDocument();
    expect(
      await within(dialog).findByRole("textbox", { name: /^code/i }),
    ).toHaveValue("");
    expect(
      await screen.findByText("On · 10 recovery codes left"),
    ).toBeInTheDocument();
  });
  it("re-asks who the session is after a removal, since a required factor confines it", async () => {
    const user = userEvent.setup();
    serve(ON, {
      "DELETE /v1/me/mfa": [() => new Response(null, { status: 204 })],
    });
    const client = mount();
    client.setQueryData(["me"], { user: { id: "u1" } });

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    await screen.findByRole("dialog");
    await user.type(codeField(), "654321{Enter}");
    await screen.findByText("Two-step sign-in removed");

    expect(client.getQueryState(["me"])?.isInvalidated).toBe(true);
  });

  it("leaves the session probe alone when a removal is refused", async () => {
    const user = userEvent.setup();
    serve(ON, { "DELETE /v1/me/mfa": [() => problem(401)] });
    const client = mount();
    client.setQueryData(["me"], { user: { id: "u1" } });

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    await screen.findByRole("dialog");
    await user.type(codeField(), "111111{Enter}");
    await screen.findByText(/code was not accepted/i);

    expect(client.getQueryState(["me"])?.isInvalidated).toBe(false);
  });
});
