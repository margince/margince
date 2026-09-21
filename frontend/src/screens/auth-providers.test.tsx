/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthScreen } from "./auth";
import { ok, render, stubApi, stubLocationAssign, t } from "./auth.testkit";
import { ProviderButtons } from "./auth-providers";

// The federated half of the sign-in surface: which providers a button exists
// for, what pressing one does, and what the card looks like at an installation
// whose only way in is a provider.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // The UI-preview switch is read from import.meta.env at the call, so a case
  // that turns it on must not leak into the next one — the default-off surface
  // is what every other case here asserts.
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
  window.location.hash = "";
});

function nameSource(button: HTMLElement): string | undefined {
  const name =
    button.querySelector(".sr-only") ??
    button.querySelector(".auth-social-label");
  return name?.textContent ?? undefined;
}

describe("federated sign-in", () => {
  it("offers a provider only when the installation serves one", async () => {
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(
      screen.queryByRole("button", { name: "Continue with Google" }),
    ).toBeNull();
    expect(screen.queryByText("or")).toBeNull();
    cleanup();

    stubApi(
      {
        password: true,
        password_reset: true,
        oidc_providers: [
          { key: "google", label: "Continue with Google" },
          { key: "microsoft", label: "Continue with Microsoft" },
        ],
      },
      () => ok(200),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(
      await screen.findByRole("button", { name: "Continue with Google" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Continue with Microsoft" }),
    ).toBeTruthy();
    // The divider labels the PASSWORD path below it, not the buttons above.
    expect(screen.getByText("or")).toBeTruthy();
  });

  // The UI-preview switch (app/ui-preview.ts), on the screen rather than on the
  // pure function. Both positions, and the OFF one is the assertion that matters:
  // every other case in this file runs with the var unset, so the default is
  // pinned by the whole suite — this pair pins that the switch is what changes it
  // and that nothing else does.
  it("draws the federated block on the real empty capability only under the UI-preview switch", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    await screen.findByLabelText("Email");
    expect(
      screen.queryByRole("button", { name: "Continue with Google" }),
    ).toBeNull();
    cleanup();

    vi.stubEnv("VITE_UI_PREVIEW_OIDC", "1");
    // Same stub, same empty `oidc_providers` the running server serves — the
    // override is presentation, so the wire is identical in both halves.
    stubApi({ password: true, password_reset: true }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    const google = await screen.findByRole<HTMLButtonElement>("button", {
      name: "Continue with Google",
    });
    expect(google.disabled).toBe(false);
    // The same switch marks the SECOND provider not-yet-available, so the preview
    // shows both halves of the design rather than two identical buttons.
    const microsoft = screen.getByRole<HTMLButtonElement>("button", {
      name: "Continue with Microsoft",
    });
    expect(microsoft.disabled).toBe(true);
    expect(microsoft.classList.contains("btn-unavailable")).toBe(true);
    // Inert, and that is the point of the switch: it draws the design, it does
    // not invent a redirect. Clicking must neither navigate nor hit the wire —
    // the navigate assertion is the one that actually matters once
    // startFederatedSignIn performs a real `location.assign`: without the
    // preview guard in front of it, this click would take the whole review
    // tab to a route the preview build never mounts.
    await stubLocationAssign(async (assign) => {
      const calls = stubApi({ password: true, password_reset: true }, () =>
        ok(200),
      );
      await userEvent.click(google);
      expect(calls).toEqual([]);
      expect(assign).not.toHaveBeenCalled();
      expect(google).toBeTruthy();
    });
  });

  // The product path, asserted as a property rather than assumed. A real server
  // can never mark a provider — `oidc_providers[]` items are `{ key, label }` with
  // no availability field — so on the shipped surface every button an
  // installation serves is live and unannotated. This is the case that fails if
  // the preview marker ever leaks into the default render.
  it("leaves every served provider enabled and unannotated, with no unavailable set", async () => {
    stubApi(
      {
        password: true,
        password_reset: true,
        oidc_providers: [
          { key: "google", label: "Continue with Google" },
          { key: "microsoft", label: "Continue with Microsoft" },
        ],
      },
      () => ok(200),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    for (const label of ["Continue with Google", "Continue with Microsoft"]) {
      const button = await screen.findByRole<HTMLButtonElement>("button", {
        name: label,
      });
      expect(button.disabled).toBe(false);
      // The accessible name is the server's label and nothing else. The role
      // query above already proves it — `name` matches the COMPUTED name, which
      // skips the `aria-hidden` copy. What is left to pin is the other half of
      // the same promise: no words of ours reach that name, and the short brand
      // word the phone layout shows is always the installation's own substring.
      expect(nameSource(button)).toBe(label);
      const brand = button.querySelector(".auth-social-brand")?.textContent;
      if (brand) {
        expect(label).toContain(brand);
      }
    }
    expect(document.querySelector(".btn-unavailable")).toBeNull();
  });

  // The preview marker (app/ui-preview.ts), on the component that renders it.
  // Passing the set explicitly rather than through the env switch is deliberate:
  // this case is about what the MARKUP does with a marked key, and the switch is
  // pinned where it lives.
  it("renders a marked provider as disabled without touching its label", async () => {
    render(
      <ProviderButtons
        providers={[
          { key: "google", label: "Continue with Google" },
          { key: "microsoft", label: "Continue with Microsoft" },
        ]}
        unavailable={new Set(["microsoft"])}
        onSelect={vi.fn()}
      />,
    );

    // The state is `Button`'s `unavailable`, which refuses the press itself and
    // draws the resting dim, and the accessible name is left as the
    // installation's own string. That is the
    // assertion worth pinning: the marker must not append copy to somebody
    // else's label, so an unrecognised provider on a real installation could
    // never have words we wrote spliced onto the words they wrote.
    const microsoft = await screen.findByRole<HTMLButtonElement>("button", {
      name: "Continue with Microsoft",
    });
    expect(microsoft.disabled).toBe(true);
    expect(microsoft.classList.contains("btn-unavailable")).toBe(true);
    // What names the button, not its raw text: the phone layout's short brand
    // word is `aria-hidden` beside an `.sr-only` copy of the served label. What
    // must never happen is a word of OURS reaching the name.
    expect(nameSource(microsoft)).toBe("Continue with Microsoft");

    // Only the marked one. The other provider is offered exactly as it would be
    // on an installation that serves it.
    const google = screen.getByRole<HTMLButtonElement>("button", {
      name: "Continue with Google",
    });
    expect(google.disabled).toBe(false);
  });

  it("renders nothing at all for an empty capability", () => {
    const { container } = render(
      <ProviderButtons providers={[]} onSelect={vi.fn()} />,
    );
    expect(container.textContent).toBe("");
  });

  // The label is the installation's string. A frontend that composed it from the
  // key would render "Continue with corp-sso" for a provider it does not know,
  // and the button still has to work for that provider — which is why the mark
  // falls back to a neutral icon rather than the block disappearing.
  it("renders an unrecognised provider with its own label and reports its key", async () => {
    const chosen: string[] = [];
    render(
      <ProviderButtons
        providers={[{ key: "corp-sso", label: "Anmeldung über Werk-IT" }]}
        onSelect={(key) => chosen.push(key)}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Anmeldung über Werk-IT" }),
    );
    expect(chosen).toEqual(["corp-sso"]);
  });

  // The real hand-off: a full-page navigation, never an XHR.
  it("navigates to the provider's start URL on click", async () => {
    await stubLocationAssign(async (assign) => {
      stubApi(
        {
          password: true,
          password_reset: true,
          oidc_providers: [{ key: "google", label: "Continue with Google" }],
        },
        () => ok(200),
      );
      render(<AuthScreen onAuthed={vi.fn()} />);

      await userEvent.click(
        await screen.findByRole("button", { name: "Continue with Google" }),
      );

      expect(assign).toHaveBeenCalledWith("/v1/auth/oidc/google/start");
    });
  });

  // A real installation's own configured provider must keep a working
  // button even if a preview build happens to run against it — the switch
  // exists to stand in for a server with NO providers, not to disable a
  // real one. Guarding on the global flag alone (rather than on whether
  // `previewedOidcProviders` actually invented this button) would make
  // this click a silent no-op on any deployment that combines the two.
  it("still navigates a real served provider even when the UI-preview switch is on", async () => {
    vi.stubEnv("VITE_UI_PREVIEW_OIDC", "1");
    await stubLocationAssign(async (assign) => {
      stubApi(
        {
          password: true,
          password_reset: true,
          oidc_providers: [{ key: "google", label: "Continue with Google" }],
        },
        () => ok(200),
      );
      render(<AuthScreen onAuthed={vi.fn()} />);

      await userEvent.click(
        await screen.findByRole("button", { name: "Continue with Google" }),
      );

      expect(assign).toHaveBeenCalledWith("/v1/auth/oidc/google/start");
    });
  });
});

// `auth.password.enabled=false`: the installation signs its members in through
// an identity provider, and a password form here would post to routes that
// refuse every submission.
describe("a closed password door", () => {
  it("draws the providers and no password form", async () => {
    stubApi(
      {
        password: false,
        password_reset: false,
        oidc_providers: [{ key: "google", label: "Continue with Google" }],
      },
      () => ok(200),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(
      await screen.findByRole("button", { name: "Continue with Google" }),
    ).toBeTruthy();
    expect(screen.queryByLabelText("Email")).toBeNull();
    expect(screen.queryByLabelText("Password")).toBeNull();
    expect(screen.queryByRole("button", { name: t("auth.signIn") })).toBeNull();
    // The divider labels the password path below it, so it goes with it.
    expect(screen.queryByText("or")).toBeNull();
  });

  // The provider is MOUNTED and not yet offering a flow — the deployment wired
  // it, the OAuth app is not stored. Boot guarantees the first and cannot
  // guarantee the second, so this state is reachable and must not be a card
  // with nothing on it.
  it("says what the installation signs in with when no provider is offered yet", async () => {
    stubApi({ password: false, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(await screen.findByText(t("auth.noMethodOffered"))).toBeTruthy();
    expect(screen.queryByLabelText("Email")).toBeNull();
  });

  // A probe that does not carry the field — an older api behind a newer
  // bundle — offers the password form, which is the method nearly every
  // installation has. Only an explicit `false` closes the door: the server
  // refuses the routes either way, so being wrong here costs a form that
  // cannot submit and never a session.
  it("offers the password form when the probe says nothing about it", async () => {
    stubApi({ password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);

    expect(await screen.findByLabelText("Email")).toBeTruthy();
  });
});
