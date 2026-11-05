/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { Mail } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import { Button } from "./atoms";
import { SERVICE_ROW_TONES, ServiceList, ServiceRow } from "./servicerow";

afterEach(cleanup);

describe("ServiceRow", () => {
  it("draws the glyph, the name, the caption and the status as a toned badge", () => {
    const { container } = render(
      <ServiceList label="Services">
        <ServiceRow
          icon={Mail}
          name="New mail"
          caption="Mail with your customers."
          status={{ tone: "success", label: "Connected" }}
        />
      </ServiceList>,
    );

    expect(container.querySelector(".service-row-icon svg")).not.toBeNull();
    expect(screen.getByText("New mail")).toBeTruthy();
    expect(screen.getByText("Mail with your customers.")).toBeTruthy();
    const badge = screen.getByText("Connected").closest(".badge");
    expect(badge?.classList.contains("badge-success")).toBe(true);
  });

  it("leaves out every slot it was not given, with no empty wrapper", () => {
    const { container } = render(
      <ServiceList label="Services">
        <ServiceRow icon={Mail} name="Read your mail" />
      </ServiceList>,
    );

    for (const part of [
      ".badge",
      ".t-caption",
      ".service-row-reason",
      ".service-row-side",
    ]) {
      expect(container.querySelector(part), part).toBeNull();
    }
  });

  it("puts the status before the one verb, and the reason under the caption", () => {
    const { container } = render(
      <ServiceList label="Services">
        <ServiceRow
          icon={Mail}
          name="New mail"
          caption="Mail with your customers."
          status={{ tone: "warning", label: "Needs attention" }}
          reason="Reconnect to go on."
          action={<Button>Reconnect</Button>}
        />
      </ServiceList>,
    );

    const side = container.querySelector(".service-row-side");
    expect(side?.firstElementChild?.classList.contains("badge")).toBe(true);
    expect(side?.lastElementChild?.textContent).toBe("Reconnect");
    const captions = container.querySelectorAll(".service-row-main .t-caption");
    expect([...captions].map((line) => line.textContent)).toEqual([
      "Mail with your customers.",
      "Reconnect to go on.",
    ]);
  });

  it("names the list and hangs each row on it as a list item", () => {
    render(
      <ServiceList label="What this account brings in">
        <ServiceRow icon={Mail} name="New mail" testId="svc-mail" />
      </ServiceList>,
    );

    const list = screen.getByRole("list", {
      name: "What this account brings in",
    });
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    expect(screen.getByTestId("svc-mail")).toBeTruthy();
  });

  it("offers no blue tone, since blue never means a status", () => {
    expect(SERVICE_ROW_TONES).not.toContain("info");
  });
});
