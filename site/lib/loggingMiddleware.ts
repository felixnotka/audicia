import { define } from "../utils.ts";

export const loggingMiddleware = define.middleware(async (ctx) => {
  const headers = ctx.req.headers;
  const method = ctx.req.method;
  const referer = headers.get("referer");
  const ipAddress = headers.get("x-forwarded-for");
  const url = ctx.url.pathname;
  const host = headers.get("host") || "";
  const userAgent = headers.get("user-agent");
  const platform = headers.get("sec-ch-ua-platform");
  const response = await ctx.next();
  const statusCode = response.status;
  const timestamp = new Date().toISOString();

  console.log(
    `[${timestamp}] ${statusCode} | ${method} ${host}${url} | from ip: ${ipAddress} | ua: ${userAgent} | on platform ${platform} | referred from ${referer}`,
  );

  return response;
});
