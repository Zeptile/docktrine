/** @type {import('tailwindcss').Config} */
const token = (name) => `rgb(var(--${name}) / <alpha-value>)`;

export default {
  content: ["./index.html", "./src/**/*.{html,js,svelte,ts}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        border: token("border"),
        input: token("input"),
        ring: token("brass"),
        background: token("background"),
        foreground: token("foreground"),
        card: { DEFAULT: token("card"), foreground: token("foreground") },
        muted: { DEFAULT: token("muted"), foreground: token("muted-foreground") },
        plate: { DEFAULT: token("plate"), deep: token("plate-deep") },
        primary: { DEFAULT: token("brass"), foreground: token("brass-ink") },
        brass: { DEFAULT: token("brass"), ink: token("brass-ink") },
        copper: token("copper"),
        iron: { DEFAULT: token("iron"), foreground: token("iron-foreground"), muted: token("iron-muted") },
        link: token("link"),
        success: token("success"),
        warning: token("warning"),
        amber: token("amber"),
        destructive: { DEFAULT: token("danger"), foreground: token("danger-ink") },
      },
      fontFamily: {
        sans: ["IBM Plex Sans", "ui-sans-serif", "system-ui", "sans-serif"],
        slab: ["Zilla Slab", "Rockwell", "Georgia", "serif"],
        mono: ["IBM Plex Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      borderRadius: {
        lg: "6px",
        md: "4px",
        sm: "2px",
      },
    },
  },
  plugins: [],
};
