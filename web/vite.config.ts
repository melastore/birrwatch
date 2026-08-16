import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  // GitHub Pages serves the site under /<repo>/, so the build needs that
  // prefix. Local builds keep the default root.
  base: process.env["VITE_BASE"] ?? "/",
  plugins: [react()],
  server: {
    // Dev server talks to birrd so the app runs the same code path in dev and
    // in production, where birrd serves the built assets itself.
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  build: { outDir: "dist", sourcemap: true },
});
