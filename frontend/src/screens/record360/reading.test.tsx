/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StoryProviders } from "../story-utils";
import { CallCard } from "./reading";

// CallCard's default head — "<name> · 360" with the sparkle mark — is what
// every caller that does not opt in must keep drawing: lead, contact and deal
// all mount this card unchanged, and a default that drifted under them would
// be a silent regression on three pages this suite never touches.

afterEach(() => {
  cleanup();
});

describe("CallCard's default head", () => {
  it("draws the kit's own subject line and mark for a caller passing no override", () => {
    render(
      <StoryProviders>
        <CallCard
          name="Aurora GmbH"
          standing={{ label: "Strong", tone: "calm" }}
        />
      </StoryProviders>,
    );
    expect(screen.getByText(/Aurora GmbH/)).toBeTruthy();
    // The mark is the head's own claim of authorship; a title override drops
    // it, so its presence here is what the default path is being held to.
    expect(document.querySelector(".co-360-mark")).toBeTruthy();
  });
});

describe("CallCard's title override", () => {
  it("draws the plain title and no sparkle mark when a caller opts in", () => {
    render(
      <StoryProviders>
        <CallCard
          name="Aurora GmbH"
          title="Account brief"
          titleAction={<span>Last update 3 Sept</span>}
          standing={{ label: "Strong", tone: "calm" }}
        />
      </StoryProviders>,
    );
    expect(screen.getByText("Account brief")).toBeTruthy();
    expect(screen.getByText("Last update 3 Sept")).toBeTruthy();
    // The kit's own subject line ("Aurora GmbH · 360") and its mark are gone:
    // the panel's own ai tone and the titleAction beside the title carry the
    // claim instead.
    expect(document.querySelector(".co-360-mark")).toBeNull();
    expect(screen.queryByText(/· 360/)).toBeNull();
  });

  it("drops titleAction along with the default head when no title is given", () => {
    // titleAction only means something beside an overriding title: the kit's
    // own head has no room for a second claim beside the one BriefTitle
    // already makes, so a caller that forgets `title` must not get a
    // titleAction floating with nothing to sit beside.
    render(
      <StoryProviders>
        <CallCard
          name="Aurora GmbH"
          titleAction={<span>Last update 3 Sept</span>}
          standing={{ label: "Strong", tone: "calm" }}
        />
      </StoryProviders>,
    );
    expect(screen.queryByText("Last update 3 Sept")).toBeNull();
  });
});
