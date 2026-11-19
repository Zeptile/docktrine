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
        ring: token("ring"),
        background: token("background"),
        foreground: token("foreground"),
        card: { DEFAULT: token("card"), foreground: token("foreground") },
        muted: { DEFAULT: token("muted"), foreground: token("muted-foreground") },
        accent: { DEFAULT: token("muted"), foreground: token("foreground") },
        primary: { DEFAULT: token("signal"), foreground: token("signal-ink") },
        signal: { DEFAULT: token("signal"), ink: token("signal-ink") },
        link: token("link"),
        success: token("success"),
        info: token("info"),
        warning: token("warning"),
        destructive: { DEFAULT: token("danger"), foreground: token("danger-ink") },
        yard: { DEFAULT: token("yard"), foreground: token("yard-foreground"), muted: token("yard-muted") },
      },
      fontFamily: {
        sans: ["Archivo Variable", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono Variable", "ui-monospace", "SFMono-Regular", "monospace"],
        stencil: ["Big Shoulders Stencil Display", "Archivo Variable", "sans-serif"],
      },
      borderRadius: {
        lg: "8px",
        md: "6px",
        sm: "3px",
      },
    },
  },
  plugins: [],
};
