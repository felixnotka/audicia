import { define } from "../../utils.ts";

export const handler = define.handlers({
  GET(ctx) {
    return ctx.redirect("/docs/getting-started/introduction");
  },
});

export default define.page(function DocsIndex() {
  return null;
});
