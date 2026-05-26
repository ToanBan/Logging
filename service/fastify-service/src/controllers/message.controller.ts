import { FastifyRequest, FastifyReply } from "fastify";
import sql from "../config/db";
import { createLogger, createRequestLogger } from "@shared/logger";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase();
const log = createLogger("fastify-service");

interface GetMessagesQuery {
  page?: string;
  limit?: string;
}

interface GetMessagesByRoomQuery {
  page?: string;
  limit?: string;
}

interface GetMessagesByRoomParams {
  room_origin_id: string;
}

export const getMessages = async (
  req: FastifyRequest<{ Querystring: GetMessagesQuery }>,
  reply: FastifyReply,
) => {
  const reqId = req.headers["x-request-id"] as string;

  const reqLog = LOG_MODE === "kafka"
    ? createRequestLogger("fastify-service", reqId)
    : log;

  if (LOG_MODE === "structured" || LOG_MODE === "kafka") {
    reqLog.info("get messages request", { req_id: reqId, extra: { query: req.query } });
  }

  const page = Math.max(1, parseInt(req.query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit || "10", 10)));
  const offset = (page - 1) * limit;

  try {
    const rows = await sql`
      SELECT * FROM chat_message
      ORDER BY created_at DESC
      LIMIT ${limit} OFFSET ${offset}
    `;

    return { success: true, pagination: { page, limit }, data: rows };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
      reqLog.error("get messages error", { req_id: reqId, extra: { error: err.message } });
    }
    reply.status(500);
    return { success: false, error: err.message };
  }
};

export const getMessagesByRoom = async (
  req: FastifyRequest<{
    Params: GetMessagesByRoomParams;
    Querystring: GetMessagesByRoomQuery;
  }>,
  reply: FastifyReply,
) => {
  const reqId = req.headers["x-request-id"] as string;
  const { room_origin_id } = req.params;

  const reqLog = LOG_MODE === "kafka"
    ? createRequestLogger("fastify-service", reqId)
    : log;

  if (LOG_MODE === "structured" || LOG_MODE === "kafka") {
    reqLog.info("get messages by room request", { req_id: reqId, extra: { room_origin_id } });
  }

  const page = Math.max(1, parseInt(req.query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit || "10", 10)));
  const offset = (page - 1) * limit;

  try {
    if (String(room_origin_id).trim() === "TRIGGER_500") {
      throw new Error("MOCK_DATABASE_CRASH: Connection timeout or deadlock simulation!");
    }

    const rows = await sql`
      SELECT * FROM chat_message
      WHERE room_origin_id = ${room_origin_id}
      ORDER BY created_at DESC
      LIMIT ${limit} OFFSET ${offset}
    `;

    if (rows.length === 0) {
      if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
        reqLog.warn("room not found", { req_id: reqId, extra: { room_origin_id } });
      }

      reply.header("connection", "keep-alive").status(404);
      return { success: false, error: `No messages found for room ${room_origin_id}` };
    }

    return { success: true, pagination: { page, limit }, data: rows };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
      reqLog.error("get messages by room error", { req_id: reqId, extra: { room_origin_id, error: err.message } });
    }
    reply.status(500);
    return { success: false, error: err.message };
  }
};