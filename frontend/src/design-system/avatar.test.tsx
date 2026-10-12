/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Avatar } from "./atoms";

// A55: the monogram is the FLOOR, not a fallback of last resort. Every company
// renders a clean mark — never a broken image, never an empty slot — so the
// initials are present in the markup whether or not a logo resolved, and a
// logo that fails to load simply stops being drawn.

afterEach(cleanup);

const ID = "company_7f3";
const LOGO = "/v1/companies/abc/logo";

function chipOf(container: HTMLElement): string {
  return container.querySelector("span")?.className ?? "";
}

// The mesh as the chip carries it: the inline numbers `.avatar-mesh` reads.
function meshOf(container: HTMLElement): string | null | undefined {
  return container.querySelector(".avatar-mesh")?.getAttribute("style");
}

describe("Avatar", () => {
  it("renders the monogram when no logo resolved", () => {
    render(<Avatar name="Voltaq Systems" identity={ID} />);
    expect(screen.getByText("VS")).toBeTruthy();
    expect(document.querySelector("img")).toBeNull();
  });

  it("draws the logo over the monogram, never instead of it", () => {
    render(<Avatar name="Voltaq Systems" identity={ID} src={LOGO} />);
    const img = document.querySelector("img");
    expect(img?.getAttribute("src")).toBe(LOGO);
    // The image is decorative: the record's name is already beside it, so a
    // screen reader announcing the mark again would only repeat the row.
    expect(img?.getAttribute("alt")).toBe("");
    // The monogram stays in the markup underneath — that is what shows until
    // the image paints.
    expect(screen.getByText("VS")).toBeTruthy();
  });

  it("falls back to the monogram when the logo fails to load", () => {
    render(<Avatar name="Voltaq Systems" identity={ID} src={LOGO} />);
    const img = document.querySelector("img");
    expect(img).toBeTruthy();
    if (img) fireEvent.error(img);
    expect(document.querySelector("img")).toBeNull();
    expect(screen.getByText("VS")).toBeTruthy();
  });

  // The monogram rule reaches names that are not two words with a space between
  // them, because the signed-in reader is frequently known to the product only
  // by their address — and a whitespace-only split turns every colleague whose
  // address begins with the same letter into the same chip.
  describe("the monogram", () => {
    it.each([
      ["Voltaq Systems", "VS"],
      ["jane.doe@example.com", "JD"],
      ["ops-team@example.com", "OT"],
      ["Ana-Sofía Ruiz", "AS"],
      ["Müller", "M"],
      ["李", "李"],
      // A plus-addressed inbox splits at the plus, not at the domain.
      ["jane+doe@example.com", "JD"],
      // "ß" uppercases to "SS"; a chip is still one letter per part.
      ["ßeta Smith", "SS"],
    ])("reads %s as %s", (name, expected) => {
      const { container } = render(<Avatar name={name} identity={ID} />);
      expect(container.textContent).toBe(expected);
      cleanup();
    });
  });

  describe("the mesh", () => {
    it("is the same for the same record every time", () => {
      const { container: first } = render(
        <Avatar name="Nordwind Energie" identity={ID} />,
      );
      const firstMesh = meshOf(first);
      expect(firstMesh).toContain("--avatar-hue-a");
      cleanup();
      const { container: second } = render(
        <Avatar name="Nordwind Energie" identity={ID} />,
      );
      expect(meshOf(second)).toBe(firstMesh);
    });

    it("differs between two records that share their initials", () => {
      const { container: first } = render(
        <Avatar name="Anna Schulz" identity="contact_1" />,
      );
      const firstMesh = meshOf(first);
      cleanup();
      const { container: second } = render(
        <Avatar name="Andreas Sommer" identity="contact_2" />,
      );
      expect(meshOf(second)).not.toBe(firstMesh);
    });

    // The key is what makes the mesh a property of the RECORD rather than of
    // the string displayed for it: a rename keeps the record's ground.
    it("follows the identity key across a rename", () => {
      const { container: before } = render(
        <Avatar identity={ID} name="Voltaq Systems" />,
      );
      const meshBefore = meshOf(before);
      cleanup();
      const { container: after } = render(
        <Avatar identity={ID} name="Voltaq Systems GmbH" />,
      );
      expect(meshOf(after)).toBe(meshBefore);
    });

    // A payload that lost its id must not hash every such chip to one colour.
    it("keys on the name when the key arrives empty", () => {
      const { container: keyed } = render(
        <Avatar identity="Voltaq Systems" name="Voltaq Systems" />,
      );
      const byName = meshOf(keyed);
      cleanup();
      const { container: empty } = render(
        <Avatar identity="" name="Voltaq Systems" />,
      );
      expect(meshOf(empty)).toBe(byName);
    });

    // A logo's chip draws no mesh at any moment — the initials wait on a
    // neutral ground — and only a logo that fails falls back to the mesh.
    it("is never drawn under a logo, and returns when the logo fails", () => {
      const { container } = render(
        <Avatar name="Voltaq Systems" identity={ID} src={LOGO} />,
      );
      const chip = container.querySelector(".avatar");
      expect(chip?.classList.contains("avatar-has-logo")).toBe(true);
      expect(meshOf(container)).toBeUndefined();
      expect(chip?.getAttribute("style")).toBeNull();
      const img = container.querySelector("img");
      if (img) fireEvent.error(img);
      expect(meshOf(container)).toContain("--avatar-hue-a");
    });
  });

  // Four sizes were being rendered — 20, 28, 44 and 76 — by three different
  // stylesheets, for a prop that admitted two. They are one scale now, and the
  // chip names its own rung rather than inheriting one from an ancestor's class.
  describe("the size", () => {
    it("is the list rung unless a caller asks otherwise", () => {
      const { container } = render(
        <Avatar name="Voltaq Systems" identity={ID} />,
      );
      expect(chipOf(container).split(" ")).toContain("avatar-sm");
    });

    it.each(["sm", "md", "lg", "xl"] as const)("names the %s rung", (size) => {
      const { container } = render(
        <Avatar name="Voltaq Systems" identity={ID} size={size} />,
      );
      expect(chipOf(container).split(" ")).toContain(`avatar-${size}`);
      cleanup();
    });
  });
});
