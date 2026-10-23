// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { StandingDims } from "./standingdims";

afterEach(cleanup);

describe("Standing dimensions", () => {
  it("carries a text sub-line whole in its tip, since the card holds it to two lines", () => {
    const sub = "of 1 open · answer the capacity question before the review";
    render(
      <StandingDims
        dims={[
          {
            key: "a",
            label: "Commitments",
            value: "1 overdue",
            sub,
            tone: "danger",
          },
        ]}
      />,
    );
    expect(screen.getByText(sub)).toHaveAttribute("title", sub);
  });

  it("gives a sub-line that is not text no tip of its own", () => {
    render(
      <StandingDims
        dims={[
          {
            key: "a",
            label: "Reach",
            value: "1 contact",
            sub: <b>Demo Admin</b>,
            tone: "neutral",
          },
        ]}
      />,
    );
    expect(screen.getByText("Demo Admin").parentElement).not.toHaveAttribute(
      "title",
    );
  });

  it("opens a card of what a reading counts, and a row of it is a link", async () => {
    const user = userEvent.setup();
    render(
      <StandingDims
        dims={[
          {
            key: "r",
            label: "Relationship",
            value: "Strong",
            sub: "3 contacts here are in touch.",
            tone: "success",
            peek: {
              on: "sub",
              head: "Contacts here who have been in touch",
              items: [
                {
                  key: "a",
                  title: "Sarah Cole",
                  sub: "last in touch 22 Oct",
                  href: "#/contacts/a",
                },
              ],
            },
          },
        ]}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Strong/ }));

    expect(screen.getByRole("link", { name: "Sarah Cole" })).toHaveAttribute(
      "href",
      "#/contacts/a",
    );
    expect(
      screen.getByRole("region", {
        name: "Contacts here who have been in touch",
      }),
    ).toBeInTheDocument();
  });

  it("draws a reading with no rows as plain words, not a button that opens nothing", () => {
    render(
      <StandingDims
        dims={[
          {
            key: "r",
            label: "Relationship",
            value: "Strong",
            tone: "success",
            peek: { head: "Contacts", items: [] },
          },
        ]}
      />,
    );

    expect(screen.queryByRole("button")).toBeNull();
  });
});
