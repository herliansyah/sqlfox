/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        app: {
          bg: '#090d16',
          surface: '#0f172a',
          card: '#1e293b',
          border: '#1e293b',
          'border-subtle': '#1e293b',
          'border-strong': '#334155',
          accent: '#38bdf8',
          danger: '#ef4444',
          warning: '#f59e0b',
          safe: '#10b981',
        },
      },
      borderColor: {
        DEFAULT: '#1e293b',
      },
      // Flat, crisp depth: no colored glows, no heavy drop shadows.
      boxShadow: {
        sm: '0 1px 2px 0 rgba(2, 6, 23, 0.55)',
        DEFAULT: '0 1px 3px 0 rgba(2, 6, 23, 0.6)',
        md: '0 4px 10px -4px rgba(2, 6, 23, 0.75)',
        lg: '0 8px 18px -10px rgba(2, 6, 23, 0.8)',
        xl: '0 12px 24px -14px rgba(2, 6, 23, 0.85)',
        '2xl': '0 16px 32px -18px rgba(2, 6, 23, 0.9)',
      },
      fontFamily: {
        mono: [
          'JetBrains Mono',
          'FiraCode Nerd Font',
          'Fira Code',
          'ui-monospace',
          'SFMono-Regular',
          'Menlo',
          'Monaco',
          'Consolas',
          'Liberation Mono',
          'Courier New',
          'monospace',
        ],
        sans: [
          'Inter',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'sans-serif',
        ],
      },
    },
  },
  plugins: [],
};
