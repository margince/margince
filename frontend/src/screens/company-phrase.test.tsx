/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { useCompanyPhrase } from "./company-phrase";

function phrase(seed: (client: QueryClient) => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false } },
  });
  seed(client);
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return renderHook(() => useCompanyPhrase(), { wrapper }).result.current;
}

it("names the company once the session read has landed", () => {
  expect(
    phrase((client) =>
      client.setQueryData(["me"], {
        ...meFixture({}),
        workspace_name: "Brandt Automotive GmbH",
      }),
    ),
  ).toBe("Brandt Automotive GmbH");
});

it("says your company before the session read lands, never an empty name", () => {
  expect(phrase(() => {})).toBe("your company");
});
