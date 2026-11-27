// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  BookingPrivacyInformation,
  bookingConsentWording,
} from "./booking-privacy";

afterEach(cleanup);

it.each(["https://example.test/privacy", "privacy@example.test"])(
  "shows the privacy address and includes it in the booking proof: %s",
  (address) => {
    render(
      <LocaleProvider initial="en">
        <BookingPrivacyInformation address={address} />
      </LocaleProvider>,
    );
    const link = screen.getByRole("link", { name: address });
    expect(link.getAttribute("href")).toBe(
      address.includes("@") ? `mailto:${address}` : address,
    );
    expect(bookingConsentWording((key) => en[key], address)).toBe(
      `${en["book.consentWording"]}\n${en["book.privacyInformation"]}: ${address}`,
    );
  },
);

it("keeps booking wording and shows no link when the installation states no address", () => {
  render(
    <LocaleProvider initial="en">
      <BookingPrivacyInformation />
    </LocaleProvider>,
  );
  expect(screen.queryByRole("link")).toBeNull();
  expect(bookingConsentWording((key) => en[key])).toBe(
    en["book.consentWording"],
  );
});

it("keeps an unsafe address as text", () => {
  render(
    <LocaleProvider initial="en">
      <BookingPrivacyInformation address="javascript:alert(1)" />
    </LocaleProvider>,
  );
  expect(screen.queryByRole("link")).toBeNull();
  expect(screen.getByText(/javascript:alert/)).toBeTruthy();
});
