import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import path from "path";

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      $lib: path.resolve("./src/lib"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["svelte"],
        },
      },
    },
  },
  server: {
    port: 5173,
    host: true,
    proxy: {
      // ws: shells are WebSockets. Host is passed through unchanged, which
      // the API's same-origin check on them relies on.
      "/api": { target: process.env.DOCKTRINE_API_PROXY ?? "http://localhost:3000", ws: true },
    },
  },
});
