/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./**/*.html",
    "./**/*.js",
    "./**/*.gohtml",
  ],
  theme: {
    extend: {},
  },
  plugins: [require('flowbite/plugin')],
};
