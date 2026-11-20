/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetTheme } from "../../testing/theme-reset";
import { AuthScreen } from "./auth";
import { ok, render, stubApi, t } from "./auth.testkit";

// The emailed reset link opens AuthScreen on its own form, and the link's
// token decides which form the reader gets.

beforeEach(resetTheme);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
  sessionStorage.clear();
});

describe("AuthScreen reset deep link", () => {
  it("redeems the emailed token and lands back at sign-in", async () => {
    const user = userEvent.setup();
    const calls = stubApi({ password: true, password_reset: true }, () =>
      ok(204),
    );
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=raw-reset-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(await screen.findByText("Password updated")).toBeTruthy();
    const request = calls[0];
    expect(String(request?.url)).toContain("/v1/auth/reset-password");
    expect(await request?.text()).toContain("raw-reset-token");
  });

  // The link is checked only when the password is submitted: the server's one
  // token verdict spends nothing before then. So the card says nothing about it.
  it("claims nothing about the link before the server has checked it", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(204));
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=unchecked-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await screen.findByLabelText("New password");
    expect(screen.getByText("Enter a new password.")).toBeTruthy();
    expect(screen.queryByText(/valid/i)).toBeNull();
  });

  it("offers a fresh link on a spent token — one neutral refusal", async () => {
    const user = userEvent.setup();
    stubApi({ password: true, password_reset: true }, () =>
      ok(401, { title: "unauthorized", detail: "invalid, used, or expired" }),
    );
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=spent-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(
      await screen.findByText(
        "This reset link is invalid, already used or expired.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Request a new link")).toBeTruthy();
  });

  it("leads back to sign-in from a refused link", async () => {
    const user = userEvent.setup();
    stubApi({ password: true, password_reset: false }, () =>
      ok(401, { title: "unauthorized", detail: "invalid, used, or expired" }),
    );
    // The real address: leaving the form rewrites it, which a stub cannot hold.
    globalThis.location.hash = "#/reset-password?token=spent-token";
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );
    await screen.findByText(t("auth.askAdminForNewLink"));
    await user.click(screen.getByRole("button", { name: "Back to sign in" }));

    expect(await screen.findByLabelText("Email")).toBeTruthy();
    expect(screen.queryByLabelText("New password")).toBeNull();
  });

  // "Request a new link" supersedes the token the user still holds. A failure
  // that is not a token verdict must not offer it, hence the absence assertions.
  it("names the action the user was actually taking when refused for rate", async () => {
    const user = userEvent.setup();
    // auth.errRateLimited speaks of sign-in attempts, and this form sets a password.
    // The 429 here keeps its own key, so the copy names the action being taken.
    stubApi({ password: true, password_reset: true }, () =>
      ok(429, { title: "rate_limited", detail: "budget exceeded" }),
    );
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=good-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(
      await screen.findByText(
        "Too many attempts. Set the password again later.",
      ),
    ).toBeTruthy();
    // The link is untouched by a rate limit, so replacing it is still wrong.
    expect(screen.queryByText("Request a new link")).toBeNull();
  });

  it("blames the password, not the link, when the server refuses the password", async () => {
    const user = userEvent.setup();
    stubApi({ password: true, password_reset: true }, () =>
      ok(422, { title: "validation_error", detail: "password too weak" }),
    );
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=good-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(
      await screen.findByText(
        "The password was refused. Choose a different password.",
      ),
    ).toBeTruthy();
    // The link is still good, so replacing it must not be offered.
    expect(screen.queryByText("Request a new link")).toBeNull();
  });

  it("keeps a good link alive when the request never reaches the server", async () => {
    const user = userEvent.setup();
    stubApi({ password: true, password_reset: true }, () => {
      throw new TypeError("Failed to fetch");
    });
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=good-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(
      await screen.findByText(
        "The password was not set. The link is still valid; retry shortly.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Request a new link")).toBeNull();
  });

  it("announces a reset failure to assistive tech", async () => {
    const user = userEvent.setup();
    // Without a role, a screen-reader user who submits an expired token hears nothing.
    stubApi({ password: true, password_reset: true }, () =>
      ok(401, { title: "unauthorized", detail: "invalid, used, or expired" }),
    );
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=spent-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    await user.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(
      "This reset link is invalid, already used or expired.",
    );
  });

  // The token does not wait in the address bar for this screen to read it.
  // lib/router.tsx takes it as it reads the hash. That keeps it out of the
  // history entry while a gate above this screen renders instead. Memory
  // outlives a mount and the address does not, so the screen empties it as it
  // takes the token. A remount that found it there would put the reset form
  // back over a reader who had gone back to sign-in.
  //
  // The REAL address, not the stubbed one the cases around this use: the scrub
  // rewrites the URL, and a plain object cannot be rewritten.
  it("does not reopen the reset form on a remount", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(204));
    globalThis.location.hash = "#/reset-password?token=good-token";
    const first = render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("New password");
    expect(globalThis.location.hash).toBe("#/reset-password");
    first.unmount();

    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(await screen.findByLabelText("Email")).toBeTruthy();
    expect(screen.queryByLabelText("New password")).toBeNull();
  });

  it("tells a signed-out reader that a reset address without its token is incomplete", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(204));
    globalThis.location.hash = "#/reset-password";
    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(await screen.findByLabelText("Email")).toBeTruthy();
    expect(screen.getByText(t("auth.noticeResetIncomplete"))).toBeTruthy();
  });

  it("ignores a token in the server-visible query string", async () => {
    // A query string reaches servers, access logs, Referer headers and Cache
    // Storage keys. So the reset view opens only for a token in the fragment.
    stubApi({ password: true, password_reset: true }, () => ok(204));
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/reset-password",
      search: "?token=raw-reset-token",
      hash: "",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);

    // The sign-in form, not the reset form.
    expect(await screen.findByLabelText("Email")).toBeTruthy();
    expect(screen.queryByLabelText("New password")).toBeNull();
  });
});
