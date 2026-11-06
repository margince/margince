/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { LinkedInChip } from "./linkedinchip";

afterEach(cleanup);

function draw(href: string, name?: string | null) {
  return render(
    <LocaleProvider>
      <LinkedInChip href={href} name={name} />
    </LocaleProvider>,
  );
}

describe("LinkedInChip names the record and opens its profile", () => {
  it("draws the record's name, not the address, and links to the profile", () => {
    draw("https://www.linkedin.com/in/dana-buyer-58b42b158", "Dana Buyer");
    const link = screen.getByRole("link", { name: "Dana Buyer on LinkedIn" });
    expect(link).toHaveTextContent("Dana Buyer");
    expect(link).not.toHaveTextContent("dana-buyer-58b42b158");
    expect(link).toHaveAttribute(
      "href",
      "https://www.linkedin.com/in/dana-buyer-58b42b158",
    );
    expect(link).toHaveAttribute("target", "_blank");
    expect(link.querySelector(".provider-mark")).not.toBeNull();
  });

  it("says LinkedIn where the record has no name", () => {
    draw("https://linkedin.com/company/brandt", "  ");
    expect(screen.getByRole("link", { name: "LinkedIn" })).toHaveTextContent(
      "LinkedIn",
    );
  });

  // The logo claims the destination; another host must not borrow it.
  it.each([
    "https://attacker.example/in/dana",
    "javascript:alert(1)",
    "in/dana-buyer",
  ])("keeps %s as plain text without the logo", (href) => {
    const { container } = draw(href, "Dana Buyer");
    expect(screen.queryByRole("link")).toBeNull();
    expect(container.querySelector(".provider-mark")).toBeNull();
    expect(container).toHaveTextContent(href);
  });
});
