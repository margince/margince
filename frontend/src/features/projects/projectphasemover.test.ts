// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import { phaseMoverSentence } from "./projectphasemover";

type Mover = components["schemas"]["PrincipalAttribution"];

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

const agent: Mover = {
  actor_type: "agent",
  actor_id: "agent:p-1",
  actor_name: null,
  on_behalf_of: "u-1",
  on_behalf_of_name: "Anna Weber",
  agent_client: "Claude",
  passport_id: "p-1",
};

describe("phaseMoverSentence", () => {
  it("names the colleague and the tool for an agent move", () => {
    expect(phaseMoverSentence(agent, false, t)).toBe(
      "Anna Weber, via Claude, moved it",
    );
  });

  it("says via an agent when the tool is not named", () => {
    expect(phaseMoverSentence({ ...agent, agent_client: null }, true, t)).toBe(
      "Anna Weber, via an agent, opened it",
    );
  });

  it("names a human by name", () => {
    expect(
      phaseMoverSentence(
        { actor_type: "human", actor_id: "human:u-1", actor_name: "Lena" },
        false,
        t,
      ),
    ).toBe("Lena moved it");
  });

  it("says System only for the system principal", () => {
    expect(
      phaseMoverSentence(
        { actor_type: "system", actor_id: "system" },
        false,
        t,
      ),
    ).toBe("System moved it");
  });

  it("reads a connector without a human as a connector", () => {
    expect(
      phaseMoverSentence(
        { actor_type: "connector", actor_id: "connector:x" },
        false,
        t,
      ),
    ).toBe("Connector moved it");
  });

  it("says unknown, never System, when no actor is recorded", () => {
    for (const unknown of [undefined, null]) {
      const text = phaseMoverSentence(unknown, false, t);
      expect(text).toBe("Who made this move was not recorded");
      expect(text).not.toContain("System");
    }
  });
});
