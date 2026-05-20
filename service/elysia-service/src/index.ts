import { Elysia } from "elysia";
import pino from "pino";
import { randomUUID } from "crypto";
import sql from "./config/db";
import { messageRoutes } from "./routes/message.routes";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase()

const logger = pino({
  level: LOG_MODE === "none" ? "silent" : (LOG_MODE === "selective" ? "warn" : "info"),
})

// Kiểm tra kết nối DB với postgres.js
try {
  await sql`SELECT 1`
  if (LOG_MODE !== "none") {
    logger.info("Database connection test successful!")
  }
} catch (err) {
  logger.error(err, "Failed to connect to database")
  process.exit(1)
}

const nopLogger = {
  info: () => {},
  warn: () => {},
  error: () => {},
  debug: () => {},
  child: () => nopLogger,
}

const app = new Elysia()
  .state({
    logger: LOG_MODE === "none" ? nopLogger as any : logger,
    reqId: "",
  })
  .onRequest(({ request, store, set }) => {
    if (LOG_MODE === "none") return

    const reqId = request.headers.get("x-request-id") || randomUUID()
    store.reqId = reqId
    store.logger = logger.child({
      req_id: reqId,
      service: "elysia-service",
    })
    set.headers["x-request-id"] = reqId
  })
  .get("/health", ({ store }) => {
    if (LOG_MODE === "structured") {
      store.logger.info({ event: "health_check" })
    }
    return { ok: true, service: "elysia-service" }
  })
  .use(messageRoutes)
  .listen(Number(process.env.PORT) || 3002)

console.log(`Elysia benchmark service running on port ${Number(process.env.PORT) || 3002}`)