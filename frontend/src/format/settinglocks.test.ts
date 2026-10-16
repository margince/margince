// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { lockFor } from "./settinglocks";

const testTranslate = (key: MessageKey, params?: Record<string, string>) =>
  translate("en", key, params);

describe("lockFor", () => {
  it("names the environment variable an operator must change", () => {
    const lock = lockFor(
      [
        {
          key: "capture.auto_enrich_daily_cap",
          origin: {
            source: "env",
            name: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
          },
        },
      ],
      "capture.auto_enrich_daily_cap",
      testTranslate,
    );
    expect(lock).toEqual({
      disabled: true,
      reason:
        "Set by MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP (environment). Change it there.",
    });
  });

  it("names the command-line flag", () => {
    const lock = lockFor(
      [{ key: "ai.budget", origin: { source: "flag", name: "--ai-budget" } }],
      "ai.budget",
      testTranslate,
    );
    expect(lock).toEqual({
      disabled: true,
      reason: "Set by --ai-budget (command line). Change it there.",
    });
  });

  it("names the variable and the .env file that holds it", () => {
    const lock = lockFor(
      [
        {
          key: "ai.budget",
          origin: {
            source: "env_file",
            name: "MARGINCE_AI_BUDGET",
            file: "/srv/margince/.env",
          },
        },
      ],
      "ai.budget",
      testTranslate,
    );
    expect(lock).toEqual({
      disabled: true,
      reason:
        "Set by MARGINCE_AI_BUDGET in /srv/margince/.env. Change it there.",
    });
  });

  it("names the key and the YAML file that holds it", () => {
    const lock = lockFor(
      [
        {
          key: "ai.budget",
          origin: {
            source: "config_file",
            name: "ai.budget",
            file: "/etc/margince/margince.yaml",
          },
        },
      ],
      "ai.budget",
      testTranslate,
    );
    expect(lock).toEqual({
      disabled: true,
      reason:
        "Set by ai.budget in /etc/margince/margince.yaml. Change it there.",
    });
  });

  it("names only the source when a file origin carries no file", () => {
    const unnamed = (source: "env_file" | "config_file") =>
      lockFor(
        [{ key: "ai.budget", origin: { source, name: "MARGINCE_AI_BUDGET" } }],
        "ai.budget",
        testTranslate,
      );
    expect(unnamed("env_file")).toEqual({
      disabled: true,
      reason: "Set by MARGINCE_AI_BUDGET (.env file). Change it there.",
    });
    expect(unnamed("config_file")).toEqual({
      disabled: true,
      reason:
        "Set by MARGINCE_AI_BUDGET (configuration file). Change it there.",
    });
  });

  it("leaves a field editable when nothing locks it", () => {
    expect(lockFor(undefined, "x", testTranslate)).toEqual({ disabled: false });
    expect(lockFor([], "x", testTranslate)).toEqual({ disabled: false });
  });

  it("leaves a field editable when the lock is on another key", () => {
    const locks = [
      { key: "y", origin: { source: "env" as const, name: "MARGINCE_Y" } },
    ];
    expect(lockFor(locks, "x", testTranslate)).toEqual({ disabled: false });
  });

  it("reads a saved, a default or an absent value as no lock", () => {
    for (const source of ["database", "default", "unset"] as const) {
      const locks = [{ key: "x", origin: { source, name: "" } }];
      expect(lockFor(locks, "x", testTranslate)).toEqual({ disabled: false });
    }
  });
});
