// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";

type Candidate = components["schemas"]["CompanyLinkCandidate"];

export const domainCandidates: Candidate[] = [
  {
    contact_id: "a1c4e7f0-2b5d-4e8a-9c1f-3d6b9e2a5c70",
    full_name: "Anja Petersen",
    title: "Head of Procurement",
    email: "anja.petersen@nordlicht-logistik.de",
  },
  {
    contact_id: "b2d5f8a1-3c6e-4f9b-8d2a-4e7c0f3b6d81",
    full_name: "Jonas Weber",
    email: "j.weber@nordlicht-logistik.de",
    employer: {
      company_id: "c3e6a9b2-4d7f-4a0c-9e3b-5f8d1a4c7e92",
      company_name: "Hanse Spedition KG",
    },
  },
  {
    contact_id: "c3e6a9b2-4d7f-4a0c-9e3b-5f8d1a4c7ea3",
    full_name: "Mira Schulte",
    title: "Fleet Manager",
    email: "mira.schulte@nordlicht-logistik.de",
  },
];

export const longDomainCandidates: Candidate[] = [
  {
    contact_id: "e5a8c1d4-6f9b-4c2e-8a5d-7b0e3f6a9c14",
    full_name:
      "Maximiliane Theresia von Hohenzollern-Sigmaringen-Wittelsbach-Brandenburg",
    title:
      "Senior Vice President of Global Strategic Procurement and Supplier Relationship Management",
    email:
      "maximiliane.theresia.von.hohenzollern-sigmaringen@nordlicht-logistik-und-lagerhaltung.de",
    employer: {
      company_id: "f6b9d2e5-7a0c-4d3f-9b6e-8c1f4a7b0d25",
      company_name:
        "Hanseatische Speditions- und Lagerhaltungsgesellschaft mit beschränkter Haftung",
    },
  },
  ...domainCandidates,
];

/** A full page as the server caps it, with more behind it. */
export const fiftyDomainCandidates: Candidate[] = Array.from(
  { length: 50 },
  (_, index) => ({
    contact_id: `d4f7b0c3-5e8a-4b1d-8f4c-${String(index).padStart(12, "0")}`,
    full_name: `Kontakt ${String(index + 1).padStart(2, "0")} Nordlicht`,
    title: index % 3 === 0 ? "Disponent" : undefined,
    email: `kontakt${index + 1}@nordlicht-logistik.de`,
  }),
);
