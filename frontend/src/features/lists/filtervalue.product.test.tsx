// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ValueControl } from "./filtervalue";

afterEach(cleanup);

function stubProducts() {
  installFetch();
}

function installFetch() {
  installFetchStub({
    "GET /me": meRoute({ product: ["read"] }),
    "GET /products": () =>
      jsonResponse({
        data: [
          { id: "pd-1", name: "Telematics" },
          {
            id: "pd-2",
            name: "Legacy bundle",
            archived_at: "2026-01-01T00:00:00Z",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
  });
}

function control(value: string | string[], onChange = vi.fn()) {
  render(
    <StoryProviders>
      <ValueControl
        type="id"
        references="product"
        options={undefined}
        op={Array.isArray(value) ? "in" : "eq"}
        value={value}
        onChange={onChange}
      />
    </StoryProviders>,
  );
  return onChange;
}

describe("a product filter value", () => {
  it("offers the catalogue's products in the picker", async () => {
    stubProducts();
    const onChange = control("");
    const user = userEvent.setup();
    await user.click(await screen.findByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Telematics" }));
    expect(onChange).toHaveBeenCalledWith("pd-1");
  });

  it("names the products a restored list clause holds, archived ones included", async () => {
    stubProducts();
    control(["pd-1", "pd-2"]);
    expect(await screen.findByText("Telematics")).toBeInTheDocument();
    expect(screen.getByText("Legacy bundle")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove Telematics" }),
    ).toBeInTheDocument();
  });
});
