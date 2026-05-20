import dotenv from "dotenv";
dotenv.config();

import Fastify, { FastifyRequest, FastifyReply } from "fastify";
import { randomUUID } from "crypto";
import sql from "./config/db";
import messageRoutes from "./routes/message.routes";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase()

const nopLogger = {
  info: () => {},
  warn: () => {},
  error: () => {},
  debug: () => {},
  trace: () => {},
  fatal: () => {},
  child: () => nopLogger,
} as any

const app = Fastify({
  logger: LOG_MODE === "none" ? false : { level: LOG_MODE === "selective" ? "warn" : "info" },
});

app.addHook("onRequest", async (req: FastifyRequest, reply: FastifyReply) => {
  const reqId = req.headers["x-request-id"]?.toString() || randomUUID();
  req.headers["x-request-id"] = reqId;

  if (LOG_MODE === "none") {
    req.log = nopLogger
  } else {
    req.log = req.log.child({
      req_id: reqId,
      service: "fastify-service",
    });
  }
});

app.get("/health", async (req: FastifyRequest, reply: FastifyReply) => {
  if (LOG_MODE === "structured") {
    req.log.info({ event: "health_check" })
  }
  return { ok: true, service: "fastify-service" }
})

app.register(messageRoutes);

const start = async () => {
  try {
    await sql`SELECT 1`

    if (LOG_MODE !== "none") {
      app.log.info("Database connection test successful!")
    }

    const port = Number(process.env.PORT) || 3001;
    await app.listen({ port, host: "0.0.0.0" });

    if (LOG_MODE !== "none") {
      app.log.info(`Fastify benchmark service running on port ${port}`)
    }
  } catch (err) {
    console.error(err);
    process.exit(1);
  }
};

start();