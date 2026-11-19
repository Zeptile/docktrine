import "@fontsource-variable/archivo/wdth";
import "@fontsource-variable/jetbrains-mono";
import "@fontsource/big-shoulders-stencil-display/latin-700";
import "@fontsource/big-shoulders-stencil-display/latin-800";
import "./app.css";
import App from "./App.svelte";
import { mount } from "svelte";

const app = mount(App, {
  target: document.getElementById("app")!,
});

export default app;
