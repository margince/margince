// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** What `/lead-signal-catalog` serves: the three factors with their bands. */
export const SIGNAL_CATALOG = {
  factors: [
    {
      factor: "web_traffic",
      company_field: null,
      bands: [
        { id: "low", points: 0, min: null, max: 9999, company_size_bands: [] },
        {
          id: "medium",
          points: 4,
          min: 10000,
          max: 100000,
          company_size_bands: [],
        },
        {
          id: "high",
          points: 8,
          min: 100001,
          max: null,
          company_size_bands: [],
        },
      ],
    },
    {
      factor: "employees",
      company_field: "size_band",
      bands: [
        {
          id: "1-10",
          points: 0,
          min: 1,
          max: 10,
          company_size_bands: ["1-10"],
        },
        {
          id: "11-50",
          points: 4,
          min: 11,
          max: 50,
          company_size_bands: ["11-50"],
        },
        {
          id: "51-200",
          points: 8,
          min: 51,
          max: 200,
          company_size_bands: ["51-200"],
        },
        {
          id: "201+",
          points: 10,
          min: 201,
          max: null,
          company_size_bands: ["201-500", "501-1000", "1001-5000", "5000+"],
        },
      ],
    },
    {
      factor: "budget_hint",
      company_field: null,
      bands: [
        {
          id: "none",
          points: -4,
          min: null,
          max: null,
          company_size_bands: [],
        },
        {
          id: "unknown",
          points: 0,
          min: null,
          max: null,
          company_size_bands: [],
        },
        { id: "some", points: 4, min: null, max: null, company_size_bands: [] },
        {
          id: "confirmed",
          points: 10,
          min: null,
          max: null,
          company_size_bands: [],
        },
      ],
    },
  ],
};
