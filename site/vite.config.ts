import { defineConfig } from "vite";
import { fresh } from "@fresh/plugin-vite";

export default defineConfig({
  plugins: [fresh()],
  define: {
    // KaTeX reads `const version = __VERSION__` at runtime; Vite must
    // replace the token at build time or the server crashes with
    // "ReferenceError: __VERSION__ is not defined".
    __VERSION__: JSON.stringify("0.16.33"),
  },
});
