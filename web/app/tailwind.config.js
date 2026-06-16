/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        diff: {
          added: '#16a34a',
          removed: '#dc2626',
          changed: '#d97706',
        },
      },
    },
  },
  plugins: [],
};
