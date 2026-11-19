/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  type Locale,
  LocaleProvider,
  storedLocale,
  useAccountLocale,
  useLocale,
} from "./index";

// A language a reader picks has to survive a reload, or the switcher reads as
// broken: the next load falls back to the browser's preference and silently
// undoes the choice. It also has to survive it WITHOUT freezing anything the
// reader did not choose — a stored browser default would outlive a change to
// the browser itself.

const STORAGE_KEY = "margince.locale";

function Probe({ adopt }: Readonly<{ adopt?: string }>) {
  const { locale, setLocale, adoptLocale } = useLocale();
  return (
    <>
      <span data-testid="locale">{locale}</span>
      <button type="button" onClick={() => setLocale("de")}>
        pick de
      </button>
      <button
        type="button"
        onClick={() =>
          // The cast is the POINT: this value comes off the wire, where the
          // compiler's guarantee ends. A server that widens the field, an older
          // release's value or a regional tag arrives here as a plain string.
          adoptLocale(adopt as Parameters<typeof adoptLocale>[0])
        }
      >
        adopt
      </button>
    </>
  );
}

const mount = (initial?: "en" | "de" | "vi", adopt?: string) =>
  render(
    <LocaleProvider initial={initial}>
      <Probe adopt={adopt} />
    </LocaleProvider>,
  );

const shown = () => screen.getByTestId("locale").textContent;

beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("a locale a reader picked", () => {
  it("is read back by the next mount, which is what a reload is", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole("button", { name: "pick de" }));
    expect(shown()).toBe("de");
    cleanup();

    mount();
    expect(shown()).toBe("de");
  });

  it("is the value that reaches storage, under a namespaced key", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole("button", { name: "pick de" }));
    expect(localStorage.getItem(STORAGE_KEY)).toBe("de");
  });

  it("yields to an explicit initial, which is the more authoritative source", () => {
    localStorage.setItem(STORAGE_KEY, "de");
    mount("vi");
    expect(shown()).toBe("vi");
  });
});

describe("a locale nobody picked", () => {
  // Storing the detected default would freeze it: a reader who later changes
  // their browser's language would keep being served the old one from a value
  // they never set.
  it("is not written to storage by a plain mount", () => {
    mount();
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("is what a stale or hand-edited stored value falls back to", () => {
    // A stored string outlives the release that wrote it, so a locale we have
    // stopped shipping must not reach the catalogs as a key they cannot serve.
    localStorage.setItem(STORAGE_KEY, "fr");
    expect(storedLocale()).toBeNull();
    mount();
    expect(shown()).not.toBe("fr");
  });

  it("is what junk in the slot falls back to", () => {
    localStorage.setItem(STORAGE_KEY, "{}");
    expect(storedLocale()).toBeNull();
  });

  it("reads as absent when nothing is stored", () => {
    expect(storedLocale()).toBeNull();
  });
});

// A locale the server reports that this release does not ship must cost the
// reader their language and nothing else. It reached `catalogs` as a key they
// had no entry for once (#2469): every lookup threw, including the error
// boundary's own, so the whole application went blank rather than one section.
describe("a locale the server reports but this release does not ship", () => {
  it("falls back instead of reaching the catalogs, and never blanks the app", async () => {
    const user = userEvent.setup();
    localStorage.setItem(STORAGE_KEY, "de");
    mount(undefined, "de-DE");

    await user.click(screen.getByRole("button", { name: "adopt" }));

    // "de", the reader's own pick on this machine — never "de-DE", which is
    // not a key the catalogs have.
    expect(shown()).toBe("de");
  });

  it("does not leave the previous account's language up after a sign-out", async () => {
    // Nobody has picked on this machine, so detection decides — and the point
    // is that an unusable answer resolves rather than being ignored, which
    // would have kept whatever the last account was reading.
    const user = userEvent.setup();
    mount("de", "de-DE");

    await user.click(screen.getByRole("button", { name: "adopt" }));

    expect(shown()).not.toBe("de-DE");
    expect(shown()).toBe("en");
  });
});

function AccountProbe({ chosen }: Readonly<{ chosen: string }>) {
  // Cast for the reason Probe's is: the account's language comes off the wire.
  useAccountLocale(chosen as Locale);
  const { locale, setLocale } = useLocale();
  return (
    <>
      <span data-testid="locale">{locale}</span>
      <button type="button" onClick={() => setLocale("de")}>
        pick de
      </button>
    </>
  );
}

// A session that ends and comes back mounts the signed-in screens again, with
// the same account language, while the provider above them stays.
function Remounting() {
  const [round, setRound] = useState(0);
  const { setLocale } = useLocale();
  return (
    <>
      <AccountProbe key={round} chosen="en" />
      <button type="button" onClick={() => setLocale("vi")}>
        pick vi
      </button>
      <button type="button" onClick={() => setRound((was) => was + 1)}>
        mount again
      </button>
    </>
  );
}

// The sign-in screen this browser shows next reads storage. So it speaks the
// language of the account last signed in here, before and after a reload.
describe("the language of the account signed in here", () => {
  it("becomes the stored choice that the next mount reads", () => {
    localStorage.setItem(STORAGE_KEY, "de");
    render(
      <LocaleProvider>
        <AccountProbe chosen="en" />
      </LocaleProvider>,
    );
    expect(shown()).toBe("en");
    expect(localStorage.getItem(STORAGE_KEY)).toBe("en");
    cleanup();
    mount();
    expect(shown()).toBe("en");
  });

  it("does not overwrite a pick made after it was adopted", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider>
        <AccountProbe chosen="en" />
      </LocaleProvider>,
    );
    await user.click(screen.getByRole("button", { name: "pick de" }));
    expect(shown()).toBe("de");
    expect(localStorage.getItem(STORAGE_KEY)).toBe("de");
    cleanup();
    mount();
    expect(shown()).toBe("de");
  });

  it("keeps a later pick stored when the signed-in screens mount again", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider>
        <Remounting />
      </LocaleProvider>,
    );
    expect(localStorage.getItem(STORAGE_KEY)).toBe("en");
    await user.click(screen.getByRole("button", { name: "pick vi" }));
    await user.click(screen.getByRole("button", { name: "mount again" }));

    expect(shown()).toBe("vi");
    expect(localStorage.getItem(STORAGE_KEY)).toBe("vi");
    cleanup();
    mount();
    expect(shown()).toBe("vi");
  });

  it("is not stored when this release does not ship it", () => {
    localStorage.setItem(STORAGE_KEY, "de");
    render(
      <LocaleProvider>
        <AccountProbe chosen="de-DE" />
      </LocaleProvider>,
    );
    expect(shown()).toBe("de");
    expect(localStorage.getItem(STORAGE_KEY)).toBe("de");
  });
});
