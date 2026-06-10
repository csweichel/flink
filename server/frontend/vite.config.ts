import react from "@vitejs/plugin-react";
import autoprefixer from "autoprefixer";
import tailwindcss from "tailwindcss";
import { defineConfig } from "vite";

export default defineConfig({
  base: "/_flink/",
  plugins: [react()],
  css: {
    postcss: {
      plugins: [
        tailwindcss({
          content: ["./index.html", "./src/**/*.{ts,tsx}"],
          theme: {
            extend: {},
          },
          plugins: [],
        }),
        autoprefixer(),
      ],
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
