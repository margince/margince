// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useHash } from "../lib/router";
import { useReportShownAddress } from "../lib/screenaddress";
import { forgetScrollMemory } from "../lib/scrollmemory";
import { ignoreSearch, render } from "../testing/shellharness";
import { Shell } from "./shell";

// The routed screen's part of the contract: it names the address it shows.
function Showing({ address }: Readonly<{ address: string }>) {
  useReportShownAddress(address);
  return <p>{address}</p>;
}

// A screen that is never held: it shows whatever the bar says.
function ShowingTheBar() {
  return <Showing address={useHash()} />;
}

function column(): HTMLElement {
  const scroller = document.querySelector<HTMLElement>(".scroll");
  if (!scroller) throw new Error("the shell drew no content column");
  return scroller;
}

function scrollColumnTo(top: number) {
  column().scrollTop = top;
  act(() => {
    column().dispatchEvent(new Event("scroll"));
  });
}

async function moveTo(hash: string, state: unknown = null) {
  await act(async () => {
    window.history.pushState(state, "", hash);
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
}

afterEach(() => {
  cleanup();
  forgetScrollMemory();
  window.history.replaceState(null, "", "#/");
});

describe("the content column's place", () => {
  it("stays where the reader left a screen still on screen past a move", async () => {
    window.history.replaceState(null, "", "#/contacts");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Showing address="#/contacts" />
      </Shell>,
    );
    scrollColumnTo(300);

    await moveTo("#/companies");

    expect(column().scrollTop).toBe(300);
  });

  it("follows an entry that differs from the last only in its dials", async () => {
    window.history.replaceState(null, "", "#/contacts?page=2");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    await moveTo("#/contacts?page=3");

    expect(column().scrollTop).toBe(0);
  });
});
