import { writable } from "svelte/store";

export type Theme = "system" | "light" | "dark";

const STORAGE = "docktrine.theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

function stored(): Theme {
  try {
    const t = localStorage.getItem(STORAGE);
    if (t === "light" || t === "dark") return t;
  } catch {}
  return "system";
}

export const theme = writable<Theme>(stored());

function apply(t: Theme) {
  const dark = t === "dark" || (t === "system" && media.matches);
  document.documentElement.classList.toggle("dark", dark);
}

theme.subscribe((t) => {
  apply(t);
  try {
    if (t === "system") localStorage.removeItem(STORAGE);
    else localStorage.setItem(STORAGE, t);
  } catch {}
});

media.addEventListener("change", () => apply(stored()));
