import { Elysia } from "elysia";
import { randomUUID } from "crypto";
import db from "./config/db";
import { messageRoutes } from "./routes/message.routes";
import { createLogger } from "@shared/logger";

const LOG_MODE = (process.env.LOG_MODE || "none")
  .trim()
  .toLowerCase();

const log = createLogger("elysia-service");

try {
  await db`SELECT 1`;
} catch (err: any) {
  log.error("failed to connect to database", {
    extra: {
      error: err.message,
    },
  });

  process.exit(1);
}

const nopLogger = {
  info: () => {},
  warn: () => {},
  error: () => {},
  debug: () => {},
};

const app = new Elysia()
  .state({
    logger: LOG_MODE === "none" ? (nopLogger as any) : log,
    reqId: "",
  })
  .onRequest(({ request, store, set }) => {
    const reqId =
      request.headers.get("x-request-id") || randomUUID();

    store.reqId = reqId;

    set.headers["x-request-id"] = reqId;

    if (LOG_MODE === "none") return;

    store.logger = log;
  })

  .get("/health", ({ store }) => {
    if (LOG_MODE === "structured") {
      store.logger.info("health check", {
        req_id: store.reqId,
      });
    }

    return {
      ok: true,
      service: "elysia-service",
    };
  })

  .use(messageRoutes)

  .listen(Number(process.env.PORT) || 3002);

