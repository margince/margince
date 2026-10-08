// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Avatar } from "./atoms";
import { CellStack } from "./cellstack";
import { Heading } from "./heading";

// Most records never get a photo or a logo, so the monogram's mesh is what a
// reader actually learns them by. This gallery is the review surface for it:
// many records side by side, at every rung, in the places a chip really sits.
const meta: Meta<typeof Avatar> = {
  title: "Components/Images and icons/Avatar",
  component: Avatar,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Avatar>;

type Sample = Readonly<{
  name: string;
  meta: string;
}>;

// The seeded records first, then invented ones: shared initials, an address-only
// reader, one word, and a script with no case to split on.
const SAMPLES: readonly Sample[] = [
  { name: "Alice Müller", meta: "Head of Purchasing · Demo GmbH" },
  { name: "Bob Schmidt", meta: "Operations · Demo GmbH" },
  { name: "Carol Wagner", meta: "CFO · Demo GmbH" },
  { name: "Demo GmbH", meta: "Wholesale · Berlin" },
  { name: "Demo Admin", meta: "Workspace owner" },
  { name: "Brandt Automotive GmbH", meta: "Automotive · Hamburg" },
  { name: "Anna Schulz", meta: "Buyer · Nordwind Energie" },
  { name: "Andreas Sommer", meta: "Plant manager · Voltaq" },
  { name: "Aylin Sahin", meta: "Procurement · Kessler Bau" },
  { name: "Maximilian Weber", meta: "Managing director" },
  { name: "Mia Wolf", meta: "Office manager" },
  { name: "Priya Nair", meta: "Chief Procurement Officer" },
  { name: "Jonas Becker", meta: "Sales engineer" },
  { name: "Lena Hoffmann", meta: "Controller" },
  { name: "Tomás García", meta: "Logistics lead" },
  { name: "Chloé Dubois", meta: "Legal counsel" },
  { name: "Oluwaseun Adeyemi", meta: "IT lead" },
  { name: "jane.doe@example.com", meta: "Known only by an address" },
  { name: "Madonna", meta: "One word" },
  { name: "李小龍", meta: "No case to split on" },
  { name: "Ana-Sofía Ruiz", meta: "Hyphenated given name" },
  { name: "van der Berg", meta: "Lower-case particle" },
  { name: "Dara O'Brien", meta: "An apostrophe in the name" },
  { name: "Müller", meta: "One word with a diacritic" },
  { name: "李", meta: "A single glyph" },
  { name: "Voltaq Systems", meta: "Industrial IoT · Munich" },
  { name: "Nordwind Energie AG", meta: "Utilities · Kiel" },
  { name: "Kessler Bau GmbH", meta: "Construction · Leipzig" },
  { name: "Kessler Bank", meta: "Finance · Frankfurt" },
  { name: "Helios Solar", meta: "Energy · Freiburg" },
  { name: "Atlas Logistik", meta: "Freight · Bremen" },
  { name: "Bergmann & Söhne", meta: "Family business · Essen" },
  { name: "Quantum Pharma", meta: "Life sciences · Basel" },
  { name: "Internationale Fahrzeugtechnik", meta: "Automotive" },
  { name: "Mühlenwerk", meta: "Food · Ulm" },
  { name: "Orbit Media", meta: "Agency · Cologne" },
  { name: "Silberstein Consulting", meta: "Advisory · Vienna" },
  { name: "Tanaka Robotics", meta: "Automation · Osaka" },
  { name: "Zeller Maschinenbau", meta: "Machinery · Stuttgart" },
  { name: "Grünwald Immobilien", meta: "Real estate · Munich" },
  { name: "Polar Data", meta: "Software · Oslo" },
  { name: "Riverside Foods", meta: "Retail · Dublin" },
  { name: "Emerald Health", meta: "Care · Zurich" },
];

const SIZES = ["sm", "md", "lg", "xl"] as const;

const column: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-6)",
};
const wall: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: "var(--space-2)",
  alignItems: "center",
};
const list: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
  gap: "var(--space-3) var(--space-6)",
};
const header: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--space-4)",
};

// Each sample's record id, which is what a real chip is keyed on.
const idOf = (sample: Sample) => `record-${SAMPLES.indexOf(sample) + 1}`;

export const Gallery: Story = {
  render: () => (
    <div style={column}>
      {SIZES.map((size) => (
        <section key={size} style={column}>
          <span className="t-label">Every record at {size}</span>
          <div style={wall}>
            {SAMPLES.map((sample) => (
              <Avatar
                key={sample.name}
                name={sample.name}
                identity={idOf(sample)}
                size={size}
              />
            ))}
          </div>
        </section>
      ))}
      <section style={column}>
        <span className="t-label">In a list row</span>
        <div style={list}>
          {SAMPLES.map((sample) => (
            <span key={sample.name} className="avatar-row">
              <Avatar name={sample.name} identity={idOf(sample)} />
              <CellStack>
                <span className="t-name">{sample.name}</span>
                <span className="t-caption">{sample.meta}</span>
              </CellStack>
            </span>
          ))}
        </div>
      </section>
      <section style={column}>
        <span className="t-label">In a record header</span>
        {SAMPLES.slice(0, 6).map((sample) => (
          <div key={sample.name} style={header}>
            <Avatar name={sample.name} identity={idOf(sample)} size="xl" />
            <CellStack>
              <Heading size="large">{sample.name}</Heading>
              <span className="t-caption">{sample.meta}</span>
            </CellStack>
          </div>
        ))}
      </section>
      <section style={column}>
        <span className="t-label">
          A logo draws no mesh: a square mark and a wide wordmark sit inside the
          circle, and only a logo that fails falls back to the mesh
        </span>
        <div style={wall}>
          <Avatar
            name="Northwind Handel"
            identity="record-logo-square"
            size="xl"
            src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect x='8' y='8' width='48' height='48' rx='10' fill='%230b7a53'/%3E%3C/svg%3E"
          />
          <Avatar
            name="Northwind Handel"
            identity="record-logo-wide"
            size="xl"
            src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 160 40'%3E%3Crect width='160' height='40' rx='6' fill='%230e7490'/%3E%3C/svg%3E"
          />
          <Avatar
            name="Northwind Handel"
            identity="record-logo-broken"
            size="xl"
            src="data:image/png;base64,AAAA"
          />
        </div>
      </section>
    </div>
  ),
};

// Keyed on the record id, so a rename does not move the mesh.
export const KeyedOnTheRecord: Story = {
  render: () => (
    <div style={wall}>
      <Avatar identity="company_7f3" name="Voltaq Systems" />
      <Avatar identity="company_7f3" name="Voltaq Systems GmbH" size="md" />
    </div>
  ),
};
