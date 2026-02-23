import { define } from "../utils.ts";

export const loggingMiddleware = define.middleware(async (ctx) => {
  const headers = ctx.req.headers;
  const method = ctx.req.method;
  const referer = headers.get("referer");
  const ipAddress = headers.get("x-forwarded-for");
  const url = ctx.url.pathname;
  const platform = headers.get("sec-ch-ua-platform");
  const response = await ctx.next();
  const statusCode = response.status;
  const timestamp = new Date().toISOString();

  console.log(
    `[${timestamp}] ${statusCode} | ${method} ${url} | from ip: ${ipAddress} | on platform ${platform} | referred from ${referer}`,
  );

  return response;
});
