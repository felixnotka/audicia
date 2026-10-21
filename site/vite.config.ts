import { defineConfig } from "vite";
import katex from "katex/package.json" with { type: "json" };
import { fresh } from "@fresh/plugin-vite";

export default defineConfig({
  plugins: [fresh()],
  define: {
    // KaTeX reads `const version = __VERSION__` at runtime; Vite must
    // replace the token at build time or the server crashes with
    // "ReferenceError: __VERSION__ is not defined". Taking the number from
    // the resolved package rather than writing it out keeps it true across
    // dependency updates, which do not touch this file.
    __VERSION__: JSON.stringify(katex.version),
  },
});
