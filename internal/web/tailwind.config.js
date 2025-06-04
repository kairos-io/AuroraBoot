/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./web/**/*.html",
    "./web/**/*.js",
    "./web/**/*.gohtml",
  ],
  theme: {
    extend: {},
  },
  plugins: [require('flowbite/plugin')],
};
