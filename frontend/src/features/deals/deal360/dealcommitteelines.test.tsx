/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { translate } from "../../../i18n";
import { CommitteeLines } from "./dealcommitteelinelist";
import { committeeLines } from "./dealcommitteelines";

type DealCoverage = components["schemas"]["DealCoverage"];

const navigate = vi.hoisted(() => vi.fn());
vi.mock("../../../lib/router", () => ({ navigate }));

const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const when = (at: string) => at.slice(0, 10);

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: "d-1",
  stakeholders: [
    {
      contact_id: "c-1",
      contact_name: "Mai Tran",
      role: "champion",
      engaged: true,
    },
    {
      contact_id: "c-2",
      contact_name: "Bao Nguyen",
      role: "user",
      engaged: false,
    },
  ],
  our_side: [
    {
      user_id: "u-1",
      display_name: "Lena Fischer",
      strength_bucket: "strong",
      interactions_90d: 4,
    },
    {
      user_id: "u-2",
      display_name: "Jonas Weber",
      strength_bucket: "weak",
      interactions_90d: 1,
    },
  ],
  risks: [],
  sections_omitted: [],
  lines: [],
  ...over,
});

afterEach(() => {
  cleanup();
  navigate.mockReset();
});

describe("committeeLines", () => {
  it("draws a line with the newest event and which way it went", () => {
    const drawn = committeeLines(
      coverage({
        lines: [
          {
            user_id: "u-1",
            contact_id: "c-1",
            last_outbound_at: "2026-11-02T09:00:00Z",
            last_inbound_at: "2026-11-04T09:00:00Z",
          },
        ],
      }),
      t,
      when,
    );
    expect(drawn?.lines.map((line) => line.text)).toEqual([
      "Lena Fischer with Mai Tran: They wrote 2026-11-04",
    ]);
  });

  it("says we wrote or we met when that is the newest event", () => {
    const drawn = committeeLines(
      coverage({
        lines: [
          {
            user_id: "u-1",
            contact_id: "c-1",
            last_outbound_at: "2026-11-02T09:00:00Z",
          },
          {
            user_id: "u-2",
            contact_id: "c-1",
            last_met_at: "2026-11-06T09:00:00Z",
          },
        ],
      }),
      t,
      when,
    );
    expect(drawn?.lines.map((line) => line.text)).toEqual([
      "Jonas Weber with Mai Tran: Met 2026-11-06",
      "Lena Fischer with Mai Tran: We wrote 2026-11-02",
    ]);
  });

  it("draws no line for a colleague who exchanged nothing with a seat", () => {
    const drawn = committeeLines(
      coverage({
        lines: [
          {
            user_id: "u-1",
            contact_id: "c-1",
            last_inbound_at: "2026-11-04T09:00:00Z",
          },
        ],
      }),
      t,
      when,
    );
    expect(drawn?.lines).toHaveLength(1);
    expect(drawn?.lines[0].text).not.toContain("Jonas Weber");
  });

  it("names a buyer no colleague has reached", () => {
    const drawn = committeeLines(
      coverage({
        lines: [
          {
            user_id: "u-1",
            contact_id: "c-1",
            last_inbound_at: "2026-11-04T09:00:00Z",
          },
        ],
      }),
      t,
      when,
    );
    expect(drawn?.unreached.map((gap) => gap.text)).toEqual([
      "No one on our team has reached Bao Nguyen",
    ]);
  });

  it("claims nothing when the read carries no lines", () => {
    expect(
      committeeLines(coverage({ lines: undefined }), t, when),
    ).toBeUndefined();
  });

  it("claims nothing when the edge sections were withheld", () => {
    expect(
      committeeLines(coverage({ sections_omitted: ["our_side"] }), t, when),
    ).toBeUndefined();
  });
});

describe("CommitteeLines", () => {
  it("lists every pair as text and opens the seat from the keyboard", async () => {
    const drawn = committeeLines(
      coverage({
        lines: [
          {
            user_id: "u-1",
            contact_id: "c-1",
            last_met_at: "2026-11-06T09:00:00Z",
          },
        ],
      }),
      t,
      when,
    );
    render(
      <CommitteeLines
        lines={drawn?.lines ?? []}
        unreached={drawn?.unreached ?? []}
      />,
    );
    const list = screen.getByRole("list");
    expect(
      screen.getByRole("region", { name: t("deal.committee.lines.title") }),
    ).toBeInTheDocument();
    const items = within(list).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Lena Fischer with Mai Tran: Met 2026-11-06",
      "No one on our team has reached Bao Nguyen",
    ]);
    await userEvent.setup().tab();
    expect(
      screen.getByRole("button", {
        name: "Lena Fischer with Mai Tran: Met 2026-11-06",
      }),
    ).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(navigate).toHaveBeenCalledWith({ screen: "contacts", id: "c-1" });
  });

  it("draws nothing when there is neither a line nor a gap", () => {
    const { container } = render(<CommitteeLines lines={[]} unreached={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
