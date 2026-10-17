import { readFile, writeFile } from "node:fs/promises";
import { defineConfig } from "vite";

export default defineConfig({
  base: "./",
  build: {
    outDir: "../cmd/taushell/frontend/dist",
    emptyOutDir: true,
  },
  plugins: [{
    name: "normalize-embedded-html",
    async closeBundle() {
      const html = new URL("../cmd/taushell/frontend/dist/index.html", import.meta.url);
      const contents = await readFile(html, "utf8");
      await writeFile(html, contents.replace(/\r\n?/g, "\n"));
    },
  }],
});
