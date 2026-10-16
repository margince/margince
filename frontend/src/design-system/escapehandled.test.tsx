/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { AiRuntimeChip, type AiRuntimeLabels } from "./airuntimechip";
import { EvidenceMark } from "./evidencemark";
import { useCloseOnEscape } from "./listmenu";
import { OverflowMenu } from "./overflowmenu";
import { Popover } from "./popover";

// An Escape a layer above already handled arrives with its default prevented,
// and every layer under it leaves that press alone: one press, one layer.

let release: (() => void) | null = null;

afterEach(() => {
  release?.();
  release = null;
  cleanup();
});

function aLayerAboveClaimsEscape() {
  const claim = (event: KeyboardEvent) => {
    if (event.key === "Escape") {
      event.preventDefault();
    }
  };
  globalThis.addEventListener("keydown", claim, true);
  release = () => globalThis.removeEventListener("keydown", claim, true);
}

const LABELS: AiRuntimeLabels = {
  configured: "Configured",
  used: "Answered by",
  route: "Route",
  calls: "Calls",
  tokens: "Tokens",
  latency: "Latency",
  estimatedCost: "Estimated cost",
  partial: "Partial",
  awaiting: "Shown after the first model call",
  unavailable: "Not available yet",
  chip: "Active model and cost",
  answering: "Answering right now",
  scope: "This run only",
};

function FilterMenu() {
  const [openKey, setOpenKey] = useState<string | null>("status");
  useCloseOnEscape(openKey, () => setOpenKey(null));
  return <output>{openKey ?? "closed"}</output>;
}

describe("a layer under an Escape already handled", () => {
  it("keeps an overflow menu open", async () => {
    const user = userEvent.setup();
    render(
      <OverflowMenu label="More actions">
        <button type="button">Merge</button>
      </OverflowMenu>,
    );
    const trigger = screen.getByRole("button", { name: "More actions" });
    await user.click(trigger);
    aLayerAboveClaimsEscape();

    await user.keyboard("{Escape}");

    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  it("keeps a popover open", async () => {
    const user = userEvent.setup();
    render(<Popover label="How it stands">Two of three are late.</Popover>);
    const trigger = screen.getByRole("button", { name: "How it stands" });
    await user.click(trigger);
    aLayerAboveClaimsEscape();

    await user.keyboard("{Escape}");

    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  it("keeps a list surface menu open", async () => {
    const user = userEvent.setup();
    render(<FilterMenu />);
    aLayerAboveClaimsEscape();

    await user.keyboard("{Escape}");

    expect(screen.getByRole("status")).toHaveTextContent("status");
  });

  it("keeps the runtime chip's popover open", async () => {
    const user = userEvent.setup();
    render(
      <AiRuntimeChip configured="ollama/gemma3" labels={LABELS} locale="en" />,
    );
    const trigger = screen.getByRole("button", { name: /Active model/ });
    await user.click(trigger);
    aLayerAboveClaimsEscape();

    await user.keyboard("{Escape}");

    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  it("keeps an evidence mark's receipt open", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <EvidenceMark
          value="1998"
          source={{
            provenance: { kind: "agent", agent: "capture" },
            snippet: "Founded in 1998",
          }}
        />
      </LocaleProvider>,
    );
    const trigger = screen.getByRole("button", { name: /1998/ });
    await user.click(trigger);
    aLayerAboveClaimsEscape();

    await user.keyboard("{Escape}");

    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });
});
