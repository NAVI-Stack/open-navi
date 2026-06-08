/** @type {import('@ladle/react').UserConfig} */
export default {
  stories: 'src/**/*.stories.{ts,tsx}',
  addons: {
    // Built-in axe-core accessibility checks (Accessibility tab + console).
    a11y: { enabled: true },
    // The component vocabulary is small + dark-themed; trim unused chrome.
    width: { enabled: false },
    rtl: { enabled: false },
  },
};
