import dotenv from "dotenv";
dotenv.config();

import Fastify, { FastifyRequest, FastifyReply } from "fastify";
import { randomUUID } from "crypto";
import sql from "./config/db";
import messageRoutes from "./routes/message.routes";
import { createLogger } from "@shared/logger";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase();
const log = createLogger("fastify-service");

const app = Fastify({ logger: false });

app.addHook("onRequest", async (req: FastifyRequest) => {
  const reqId = req.headers["x-request-id"]?.toString() || randomUUID();
  req.headers["x-request-id"] = reqId;
});

app.get("/health", async (req: FastifyRequest) => {
  if (LOG_MODE === "structured") {
    log.info("health check", { req_id: req.headers["x-request-id"] as string });
  }
  return { ok: true, service: "fastify-service" };
});

app.register(messageRoutes);

const start = async () => {
  try {
    await sql`SELECT 1`;
    const port = Number(process.env.PORT) || 3001;
    await app.listen({ port, host: "0.0.0.0" });
  } catch (err: any) {
    log.error("server failed to start", { extra: { error: err.message } });
    process.exit(1);
  }
};

start();