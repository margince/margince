/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider, useT } from ".";
import { usePublicLocale } from "./publiclocale";

// The language a public page speaks, taken from the link that opened it.

function Page() {
  usePublicLocale();
  return <p>{useT()("prefs.title")}</p>;
}

function renderAt(hash: string) {
  globalThis.location.hash = hash;
  return render(
    <LocaleProvider>
      <Page />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
  globalThis.location.hash = "";
  globalThis.localStorage.clear();
});

describe("the language a public link carries", () => {
  it("speaks the language the link named", async () => {
    renderAt("#/preferences/tok?lang=de");
    expect(
      await screen.findByText("Wähle, was du von uns hörst"),
    ).toBeDefined();
  });

  // ?lang is what the MESSAGE happened to be written in, not a choice this
  // reader made — so it must not become their stored pick and follow them
  // onto every other screen.
  it("does not store the link's language as the reader's own choice", async () => {
    renderAt("#/preferences/tok?lang=de");
    await screen.findByText("Wähle, was du von uns hörst");
    expect(globalThis.localStorage.getItem("margince.locale")).toBeNull();
  });

  it("ignores a language this product does not speak", async () => {
    globalThis.localStorage.setItem("margince.locale", "de");
    renderAt("#/preferences/tok?lang=klingon");
    // The stored pick still decides, rather than the page falling back to
    // English because the link said something unrecognised.
    expect(
      await screen.findByText("Wähle, was du von uns hörst"),
    ).toBeDefined();
  });

  it("leaves the stored pick alone when the link names no language", async () => {
    globalThis.localStorage.setItem("margince.locale", "de");
    renderAt("#/preferences/tok");
    expect(
      await screen.findByText("Wähle, was du von uns hörst"),
    ).toBeDefined();
    expect(globalThis.localStorage.getItem("margince.locale")).toBe("de");
  });
});
