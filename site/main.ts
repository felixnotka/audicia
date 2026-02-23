import { App, staticFiles } from "fresh";
import { chartsMiddleware } from "./lib/chartsMiddleware.ts";
import { loggingMiddleware } from "./lib/loggingMiddleware.ts";

export const app = new App()
  .use(chartsMiddleware)
  .use(staticFiles())
  .use(loggingMiddleware)
  .onError("*", (ctx) => {
    console.log(`Error: ${ctx.error}`);
    return new Response("Oops!", { status: 500 });
  })
  .notFound(() => {
    return new Response("Not Found", { status: 404 });
  });

app.fsRoutes();
