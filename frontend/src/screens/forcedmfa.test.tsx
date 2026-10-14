/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ForcedMfaScreen } from "./forcedmfa";

const SECRET = "JBSWY3DPEHPK3PXP";
const CODES = ["aB3-dEf_ghIJkl", "Zx9_yW8-vU7tsr"];

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Answers each route from its queue and records what was asked for.
function serve(routes: Record<string, (() => Response)[]>) {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const route = `${request.method} ${new URL(request.url).pathname}`;
      asked.push(route);
      return routes[route]?.shift()?.() ?? json({ status: 500 }, 500);
    }),
  );
  return asked;
}

function mount(onEnrolled = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ForcedMfaScreen onEnrolled={onEnrolled} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return onEnrolled;
}

const enrolment = () =>
  json({
    secret: SECRET,
    otpauth_uri: `otpauth://totp/Margince:a%40b.test?secret=${SECRET}`,
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the forced two-step setup", () => {
  it("runs the setup, shows the recovery codes, and opens the app on Done", async () => {
    const user = userEvent.setup();
    const asked = serve({
      "POST /v1/me/mfa/totp": [enrolment],
      "POST /v1/me/mfa/totp/confirm": [() => json({ recovery_codes: CODES })],
    });
    const onEnrolled = mount();

    expect(screen.getByText(/requires two-step sign-in/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Set up" }));
    expect(
      await screen.findByRole("img", { name: /qr code/i }),
    ).toBeInTheDocument();
    await user.type(
      screen.getByRole("textbox", { name: /code from the app/i }),
      "123 456{Enter}",
    );
    expect(
      await screen.findByRole("textbox", { name: "Recovery codes" }),
    ).toHaveTextContent(CODES[0]);
    expect(screen.queryByRole("button", { name: "Sign out" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(onEnrolled).toHaveBeenCalledOnce();
    expect(asked).toEqual([
      "POST /v1/me/mfa/totp",
      "POST /v1/me/mfa/totp/confirm",
    ]);
  });

  it("signs out without setting anything up", async () => {
    const user = userEvent.setup();
    const asked = serve({
      "POST /v1/auth/logout": [() => new Response(null, { status: 204 })],
    });
    const onEnrolled = mount();

    await user.click(screen.getByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(asked).toContain("POST /v1/auth/logout"));
    expect(asked).not.toContain("POST /v1/me/mfa/totp");
    expect(onEnrolled).not.toHaveBeenCalled();
  });

  it("lets an account that enrolled elsewhere continue", async () => {
    const user = userEvent.setup();
    serve({
      "POST /v1/me/mfa/totp": [
        () =>
          json(
            { title: "Conflict", status: 409, code: "mfa_already_enrolled" },
            409,
          ),
      ],
    });
    const onEnrolled = mount();

    await user.click(screen.getByRole("button", { name: "Set up" }));
    await user.click(await screen.findByRole("button", { name: "Continue" }));
    expect(onEnrolled).toHaveBeenCalledOnce();
  });
});
