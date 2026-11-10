/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./web/src/**/*.{html,js}",
    "./web/dist/**/*.{html,js}"
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        app: {
          bg: '#090d16',
          surface: '#0f172a',
          card: '#1e293b',
          border: '#334155',
          accent: '#38bdf8',
          danger: '#ef4444',
          warning: '#f59e0b',
          safe: '#10b981'
        }
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
          'monospace'
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
          'sans-serif'
        ]
      }
    }
  },
  plugins: [],
}
