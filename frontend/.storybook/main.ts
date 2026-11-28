import type { StorybookConfig } from "@storybook/react-vite";

// Storybook on the app's own Vite. Stories are the render surface the
// change-scoped capture gate, frontend/scripts/fe-uat.mjs, drives.
const config: StorybookConfig = {
  stories: ["../src/**/*.mdx", "../src/**/*.stories.@(ts|tsx)"],
  addons: ["@storybook/addon-docs", "@storybook/addon-a11y"],
  framework: { name: "@storybook/react-vite", options: {} },
  // Test mode keeps the MDX page, its blocks, the docs addon and autodocs.
  // So it registers and checks what a full build does; only docgen and tree-shaking go.
  build: {
    test: {
      disableMDXEntries: false,
      disabledAddons: [],
      disableBlocks: false,
      disableAutoDocs: false,
    },
  },
  // Storybook keeps the app's plugins but not its chunk groups, so the app's
  // chunk budgets would find no Home chunk here.
  viteFinal: (vite) => ({
    ...vite,
    plugins: (vite.plugins ?? [])
      .flat()
      .filter(
        (plugin) =>
          !(plugin && "name" in plugin && plugin.name === "chunk-budgets"),
      ),
  }),
};

export default config;
