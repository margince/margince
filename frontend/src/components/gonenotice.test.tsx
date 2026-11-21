// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { QueryStates } from "../design-system/querystates";
import { LocaleProvider, translate } from "../i18n";
import { ProblemError } from "../lib/problem";
import { GoneNotice } from "./gonenotice";

afterEach(cleanup);

it("says a 404 or 410 link is gone in the screen's sentence, with its way on and no Retry", () => {
  for (const status of [404, 410]) {
    const { unmount } = render(
      <LocaleProvider initial="en">
        <QueryStates
          query={{
            isPending: false,
            isError: true,
            error: new ProblemError({ status, detail: "not found" }),
            refetch: () => undefined,
          }}
          pendingLabel="Loading the section"
          gone={
            <GoneNotice
              messageKey="book.linkGone"
              action={<a href="#back">Back</a>}
            />
          }
        >
          {null}
        </QueryStates>
      </LocaleProvider>,
    );
    expect(screen.getByRole("alert").textContent).toBe(
      translate("en", "book.linkGone"),
    );
    expect(
      screen.queryByRole("button", { name: translate("en", "common.retry") }),
    ).toBeNull();
    expect(screen.getByText("Back")).toBeTruthy();
    unmount();
  }
});
