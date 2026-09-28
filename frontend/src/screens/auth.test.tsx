/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { THEME_KEY } from "../app/theme";
import { resetTheme } from "../app/theme-reset";
import { LOCALES, localeNameKey, translate } from "../i18n";
import { AuthScreen, AvailabilityScreen } from "./auth";
import { ok, render, stubApi, t } from "./auth.testkit";

// The unauthenticated surface (A107/ADR-0061 §12): login is the default —
// no signup mode, no workspace field, no tenant selector on the wire — and
// the forgot-password flow renders exactly when the capabilities probe
// reports it operational.

// The theme lives in one module-level store, so the case that presses the
// toggle below would otherwise hand every later case a flipped document,
// `localStorage` and store.
beforeEach(resetTheme);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // The UI-preview switch is read from import.meta.env at the call, so a case
  // that turns it on must not leak into the next one — the default-off surface
  // is what every other case in this file asserts.
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
  window.location.hash = "";
});

describe("AuthScreen login", () => {
  it("introduces Margince in two sentences and claims nothing else", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    // The greeting is TYPED (ADR-0076 Decision 5), so the visible layer holds a
    // partial string for the first second and there are three nodes carrying it.
    // Assert on the `.sr-only` one: it is what a screen reader is handed, it is
    // complete on the first render, and reading the visible layer instead would
    // be asserting on a race.
    expect(
      screen.getByText("This is Margince.", { selector: ".sr-only" }),
    ).toBeTruthy();
    expect(
      screen.getByText("It takes care of the work around your work."),
    ).toBeTruthy();
    // What the region no longer says, asserted because each was removed on
    // purpose and a silent return would be a change nobody asked for: the
    // disclosure kicker that named the region, the send promise, the handover,
    // and the installation's own AI posture — which this screen showed to
    // anybody who could load it.
    expect(screen.queryByText("Margince · AI system")).toBeNull();
    expect(screen.queryByText(/never send an email or message/)).toBeNull();
    expect(screen.queryByText(/really you/)).toBeNull();
    expect(screen.queryByText(/Configured|routing/)).toBeNull();
  });

  // Derived from LOCALES rather than listed: a hardcoded pair passes while the
  // footer quietly drops the third language, which is the one failure this
  // case exists to catch — the reader who cannot read the screen it is on.
  it("the sign-in footer offers every shipped locale", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    for (const locale of LOCALES) {
      const name = t(localeNameKey(locale));
      expect(screen.getByRole("button", { name }), name).toBeTruthy();
    }
  });

  // The document declares ONE language (LocaleProvider, WCAG 3.1.1) and this
  // row shows three. Unmarked, a screen reader reads every name with the
  // phonemes of whichever locale is currently on — so the reader who came here
  // to find their own language is read it in a language they may not have.
  it("voices each language name in its own language", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    for (const locale of LOCALES) {
      const name = t(localeNameKey(locale));
      const button = screen.getByRole("button", { name });
      expect(
        button.querySelector(`[lang="${locale}"]`)?.textContent,
        name,
      ).toBe(name);
    }
  });

  it("is a login form — no signup mode, no workspace field, Enter submits, no tenant header", async () => {
    const calls = stubApi({ password: true, password_reset: false }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    const onAuthed = vi.fn();
    render(<AuthScreen onAuthed={onAuthed} />);

    expect(screen.queryByLabelText(/workspace/i)).toBeNull();
    expect(
      screen.queryByText(/create (your )?workspace|create one|sign up/i),
    ).toBeNull();

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    // Enter inside the real <form> submits — no button click needed.
    await userEvent.type(
      screen.getByLabelText("Password"),
      "correct-horse-battery{enter}",
    );

    await waitFor(() => expect(onAuthed).toHaveBeenCalled());
    const request = calls[0];
    expect(String(request?.url)).toContain("/v1/auth/login");
    expect(request?.headers.has("X-Workspace-Slug")).toBe(false);
  });

  it("does not show success until the authenticated session probe succeeds", async () => {
    stubApi({ password: true, password_reset: false }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    const probe = vi.fn().mockRejectedValue(new Error("session rejected"));
    const { container } = render(<AuthScreen onAuthed={probe} />);

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    await userEvent.type(
      screen.getByLabelText("Password"),
      "correct-horse-battery{enter}",
    );

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Margince could not be reached",
    );
    expect(probe).toHaveBeenCalledOnce();
    expect(
      container.querySelector<HTMLElement>(".auth-surface")?.dataset.authPhase,
    ).toBe("error");
  });

  it("answers bad credentials with the one non-enumerating message, keeps the email, clears the password", async () => {
    stubApi({ password: true, password_reset: false }, () =>
      ok(401, {
        title: "unauthorized",
        detail: "invalid email or password",
      }),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "wrong{enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(
      "Sign-in failed. Check the email and password and retry.",
    );
    expect(screen.getByLabelText("Email")).toHaveProperty(
      "value",
      "ada@example.com",
    );
    // §9.2: a rejected credential clears the password for the retry.
    expect(screen.getByLabelText("Password")).toHaveProperty("value", "");
  });

  it("presents rate limiting as its own actionable state, never a credential error", async () => {
    stubApi({ password: true, password_reset: false }, () =>
      ok(429, { title: "budget exceeded" }),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "whatever{enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(
      "Too many sign-in attempts. Retry later.",
    );
  });

  it("presents a server outage as connectivity, not wrong credentials", async () => {
    stubApi({ password: true, password_reset: false }, () =>
      ok(500, { title: "boom" }),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "whatever{enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Margince could not be reached");
  });

  it("restores a deep link after login instead of forcing the Brief", async () => {
    stubApi({ password: true, password_reset: false }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    window.location.hash = "#/deals/d-42";
    render(<AuthScreen onAuthed={vi.fn()} />);

    await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
    await userEvent.type(
      screen.getByLabelText("Password"),
      "correct-horse-battery{enter}",
    );

    await waitFor(() => expect(window.location.hash).toBe("#/deals/d-42"));
  });

  it("renders the session notices the boundary hands it", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} notice="session-expired" />);
    expect(
      await screen.findByText(
        "The session expired. Sign in again to continue.",
      ),
    ).toBeTruthy();
    cleanup();

    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} notice="signed-out" />);
    expect(await screen.findByText("You have been signed out.")).toBeTruthy();
  });

  it("hides the forgot-password link when the capability is off, shows it when on", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(screen.queryByText("Forgot password?")).toBeNull();
    cleanup();

    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText("Forgot password?")).toBeTruthy();
  });

  // The reset UI-preview switch (app/ui-preview.ts), on the screen. The capability
  // is `false` in both halves — the running installation's own answer, since it
  // has no mailer — so the switch is the only difference, which is the property
  // this pair exists to pin.
  it("draws the forgot-password link on a false capability only under the UI-preview switch", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(screen.queryByText("Forgot password?")).toBeNull();
    cleanup();

    vi.stubEnv("VITE_UI_PREVIEW_RESET", "1");
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText("Forgot password?")).toBeTruthy();
  });

  // §12: the two fields keep their VISIBLE labels, which is where this build
  // deliberately parts company with the reference artifact — it names its fields
  // with a placeholder and an aria-label. A placeholder is not a label: it
  // disappears the moment the field has content (WCAG 3.3.2). The bordered shell
  // must not quietly move the accessible name onto itself either.
  it("names both fields with a real label, not a placeholder", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    for (const name of ["Email", "Password"]) {
      const field = await screen.findByLabelText(name);
      expect(field.tagName).toBe("INPUT");
      // The accessible name comes from the <label>, so it survives typing.
      expect(field.getAttribute("aria-label")).toBeNull();
    }
  });

  // §6.7: the legal line states that ACCESS is restricted — never that data is
  // safe, encrypted, sovereign or compliant, because those are outcome claims the
  // installation's own configuration can contradict (VOICE-RULE-7).
  it("states that access is restricted, and nothing about the data", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(
      await screen.findByText("Access to this company is restricted."),
    ).toBeTruthy();
    expect(
      screen.queryByText(/encrypted|compliant|sovereign|your data is safe/i),
    ).toBeNull();
    // Server paths, not app routes: both documents have to be readable BEFORE
    // anyone authenticates, so they cannot sit behind the SPA router.
    expect(screen.getByRole("link", { name: "Terms" })).toHaveProperty(
      "pathname",
      "/legal/terms",
    );
    expect(screen.getByRole("link", { name: "Privacy" })).toHaveProperty(
      "pathname",
      "/legal/privacy",
    );
  });

  // The theme is readable before anyone signs in, so it has to be changeable
  // there too — the toggle used to exist only in the authenticated top bar.
  it("changes the document theme from the legal row", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    const toggle = await screen.findByRole("button", {
      name: t("theme.toDark"),
    });
    await userEvent.click(toggle);

    expect(document.documentElement.dataset.theme).toBe("dark");
    // The label names the theme the press would move TO, so it has to flip with
    // the press — a stale label sends a reader the wrong way.
    expect(toggle.getAttribute("aria-label")).toBe(t("theme.toLight"));
    expect(window.localStorage.getItem(THEME_KEY)).toBe("dark");
  });
});

// §19/§11, and now the markup exists — so the gate has to be the CAPABILITY
// rather than the absence of a component. Both directions, because only ever
// testing the empty case is what let the block go unbuilt for so long.
/**
 * The text that NAMES a provider button.
 *
 * A button carrying the phone layout's short brand word has two label spans: an
 * `.sr-only` copy of the served label, which is what assistive tech reads, and an
 * `aria-hidden` visible one. A button whose served label has no recognised brand
 * word has a single span and no `.sr-only` copy. Reading whichever exists is how
 * these tests assert the name without depending on which layout the button was
 * rendered for.
 */
describe("OIDC failure notice", () => {
  it("shows a neutral notice when the address carries the callback's failure marker, then scrubs it", async () => {
    window.location.hash = "#/login?oidc=failed";
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(await screen.findByText(t("auth.noticeOidcFailed"))).toBeTruthy();
    expect(window.location.hash).toBe("");
  });

  it("stays silent for an ordinary address", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(screen.queryByText(t("auth.noticeOidcFailed"))).toBeNull();
  });

  // A later, unrelated remount of AuthScreen within the same page — a
  // session expiring and sending the reader back to login, say — must not
  // replay a marker that was already consumed and scrubbed from the
  // address. This is the exact case a page-lifetime memo would get wrong:
  // it would keep answering the FIRST mount's verdict forever.
  it("does not replay the notice on a later, unrelated mount", async () => {
    window.location.hash = "#/login?oidc=failed";
    stubApi({ password: true, password_reset: true }, () => ok(200));
    const { unmount } = render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText(t("auth.noticeOidcFailed"))).toBeTruthy();
    unmount();

    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(screen.queryByText(t("auth.noticeOidcFailed"))).toBeNull();
  });
});

describe("AuthScreen forgot password", () => {
  it("requests the reset and confirms neutrally", async () => {
    const calls = stubApi({ password: true, password_reset: true }, () =>
      ok(202),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    await userEvent.click(await screen.findByText("Forgot password?"));
    await userEvent.type(
      screen.getByLabelText("Email"),
      "ada@example.com{enter}",
    );

    expect(await screen.findByText("Check your email")).toBeTruthy();
    expect(String(calls[0]?.url)).toContain("/v1/auth/forgot-password");
  });
});

// Every password field on this surface, DERIVED from the rendered form rather
// than named one by one: the autocomplete token is what makes a field a password
// field to a browser and to a password manager, and it survives the reveal (the
// `type` does not). A field this surface grows later is therefore covered by the
// obligation without anyone extending a list — which is the whole point, because
// a missing reveal looks exactly like a field nobody got round to.
async function expectEveryPasswordFieldRevealable() {
  const fields = [
    ...document.querySelectorAll<HTMLInputElement>(
      'input[autocomplete$="-password"]',
    ),
  ];
  expect(fields.length).toBeGreaterThan(0);
  for (const input of fields) {
    const field = input.closest(".field");
    expect(field, input.name).toBeTruthy();
    const reveal = field?.querySelector<HTMLButtonElement>(".field-reveal");
    expect(reveal, input.name).toBeTruthy();
    expect(input.type, input.name).toBe("password");
    expect(reveal?.getAttribute("aria-label")).toBe("Show password");
    expect(reveal?.getAttribute("aria-pressed")).toBe("false");

    if (reveal) {
      await userEvent.click(reveal);
    }
    expect(input.type, input.name).toBe("text");
    expect(reveal?.getAttribute("aria-label")).toBe("Hide password");
    expect(reveal?.getAttribute("aria-pressed")).toBe("true");

    if (reveal) {
      await userEvent.click(reveal);
    }
    expect(input.type, input.name).toBe("password");
  }
}

describe("password reveal", () => {
  it("covers the sign-in password", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Password");

    await expectEveryPasswordFieldRevealable();
  });

  // The one that was missing, and the worse of the two to get wrong: a mistyped
  // sign-in password is refused by the server, while a mistyped NEW password just
  // becomes the password — there is no confirm field to disagree with it.
  it("covers the new password behind the emailed link", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(204));
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/",
      hash: "#/reset-password?token=reveal-probe-token",
      origin: "http://localhost",
    });
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("New password");

    await expectEveryPasswordFieldRevealable();
  });
});

describe("AuthScreen reset deep link", () => {
  it("redeems the emailed token and lands back at sign-in", async () => {
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

    await userEvent.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    expect(await screen.findByText("Password updated")).toBeTruthy();
    const request = calls[0];
    expect(String(request?.url)).toContain("/v1/auth/reset-password");
    expect(await request?.text()).toContain("raw-reset-token");
  });

  it("offers a fresh link on a spent token — one neutral refusal", async () => {
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

    await userEvent.type(
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

  // These three cases exist because one string used to serve every failure, and
  // the remedy it offered — "Request a new link" — SUPERSEDES the token the user
  // is still holding. So for anything that is not a token verdict, the old copy
  // was both wrong and destructive. What each failure must not do is as important
  // as what it says, hence the absence assertions.
  it("names the action the user was actually taking when refused for rate", async () => {
    // The 429 used to render auth.errRateLimited, which says "sign-in attempts"
    // — on a form whose only job is setting a password. Copy that names the wrong
    // action reads as the wrong error, so this branch has its own key and this
    // test is what keeps the two from being collapsed back together.
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

    await userEvent.type(
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

    await userEvent.type(
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

    await userEvent.type(
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
    // The reset error was the one error region on this surface with no role, so a
    // screen-reader user submitted an expired token and was told nothing.
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

    await userEvent.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(
      "This reset link is invalid, already used or expired.",
    );
  });

  // The token no longer waits in the address bar for this screen to read it:
  // app/router.tsx takes it as it reads the hash, which is what keeps it out of
  // the history entry while a gate above this screen renders instead. Memory
  // outlives a mount and the address did not, so the screen empties it as it
  // takes the token in hand — a remount that found it there would put the reset
  // form back over a reader who had gone back to sign-in.
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

  it("ignores a token in the server-visible query string", async () => {
    // The security property, asserted directly rather than implied by the happy
    // path: a query string is sent to servers, lands in access logs, is attached
    // as a Referer on same-origin api calls, and becomes a Cache Storage key. So
    // the reset view must open ONLY for a fragment token. If someone reverts the
    // parser to location.search to "be more forgiving", this fails.
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

describe("AvailabilityScreen", () => {
  it("presents connectivity and installation problems as availability with a retry", async () => {
    const onRetry = vi.fn();
    render(<AvailabilityScreen kind="connection" onRetry={onRetry} />);
    expect(screen.getByText("Margince could not be reached")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalled();
    cleanup();

    render(<AvailabilityScreen kind="installation" onRetry={vi.fn()} />);
    expect(screen.getByText("Installation not ready")).toBeTruthy();
    // No credential fields: this is not a login problem.
    expect(screen.queryByLabelText("Email")).toBeNull();
  });

  // The refusal must add no signal an attacker can probe with. It is one static
  // catalog string with no interpolation slot, so an unknown address, an
  // invited one and an active one all produce the same words — the same reason
  // /auth/login refuses without saying which half was wrong.
  //
  // Held as a property of the STRING rather than by rendering three accounts:
  // the notice is driven by the redirect's hash and never by a response, so a
  // render-based test would pass no matter what the server had said.
  it("says the same thing whichever account was tried", () => {
    for (const locale of LOCALES) {
      const notice = translate(locale, "auth.noticeOidcFailed");
      expect(notice).not.toMatch(/\{[^}]+\}/);
      // And it still tells the one contact who is genuinely stuck what to do.
      expect(notice.length).toBeGreaterThan(40);
    }
  });
});
