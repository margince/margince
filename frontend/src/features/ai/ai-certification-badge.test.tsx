/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { CertificationBadge } from "./ai-certification-badge";

type Certification = components["schemas"]["AiTaskCertification"];

afterEach(cleanup);

function renderBadge(certification: Certification | undefined) {
  return render(
    <LocaleProvider initial="en">
      <CertificationBadge certification={certification} />
    </LocaleProvider>,
  );
}

describe("CertificationBadge", () => {
  it.each([
    ["ready", "Ready"],
    ["usable_with_care", "Usable with care"],
    ["not_reliable", "Not reliable yet"],
    ["not_measured", "Not measured"],
  ] as const)("names grade %s as %s", (grade, label) => {
    renderBadge({
      grade,
      model: "gemini-3.1-flash-lite",
      recheck_pending: false,
    });
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.getByTitle(/gemini-3\.1-flash-lite/)).toBeInTheDocument();
  });

  it("says when the grade waits on a re-check", () => {
    renderBadge({ grade: "ready", model: "m", recheck_pending: true });
    expect(screen.getByText("Ready, re-check pending")).toBeInTheDocument();
  });

  it("reads a grade it does not know as not measured, never as ready", () => {
    renderBadge({
      grade: "graded_by_a_newer_server" as Certification["grade"],
      model: "m",
      recheck_pending: false,
    });
    expect(
      screen.getByText(englishCopy["aiCert.notMeasured"]),
    ).toBeInTheDocument();
  });

  it("draws nothing for a task with no bound model", () => {
    const { container } = renderBadge(undefined);
    expect(container).toBeEmptyDOMElement();
  });
});
