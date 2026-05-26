import db from "../config/db";
import { createLogger, createRequestLogger } from "@shared/logger";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase();
const log = createLogger("elysia-service");

export const getMessages = async ({ query, store, set }: any) => {
  const reqId = store.reqId as string;

  const reqLog = LOG_MODE === "kafka"
    ? createRequestLogger("elysia-service", reqId)
    : log;

  if (LOG_MODE === "structured" || LOG_MODE === "kafka") {
    reqLog.info("get messages request", { req_id: reqId, extra: { query } });
  }

  const page = Math.max(1, parseInt(query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(query.limit || "10", 10)));
  const offset = (page - 1) * limit;

  try {
    const rows = await db`
      SELECT * FROM chat_message
      ORDER BY created_at DESC
      LIMIT ${limit} OFFSET ${offset}
    `;

    return { success: true, pagination: { page, limit }, data: rows };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
      reqLog.error("get messages error", { req_id: reqId, extra: { error: err.message } });
    }
    set.status = 500;
    return { success: false, error: err.message };
  }
};

export const getMessagesByRoom = async ({ params, query, store, set }: any) => {
  const reqId = store.reqId as string;
  const { room_origin_id } = params;

  const reqLog = LOG_MODE === "kafka"
    ? createRequestLogger("elysia-service", reqId)
    : log;

  if (LOG_MODE === "structured" || LOG_MODE === "kafka") {
    reqLog.info("get messages by room request", { req_id: reqId, extra: { room_origin_id } });
  }

  const page = Math.max(1, parseInt(query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(query.limit || "10", 10)));
  const offset = (page - 1) * limit;

  try {
    const rows = await db`
      SELECT * FROM chat_message
      WHERE room_origin_id = ${room_origin_id}
      ORDER BY created_at DESC
      LIMIT ${limit} OFFSET ${offset}
    `;

    if (rows.length === 0) {
      if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
        reqLog.warn("room not found", { req_id: reqId, extra: { room_origin_id } });
      }

      set.status = 404;
      set.headers = { connection: "keep-alive" };
      return { success: false, error: `No messages found for room ${room_origin_id}` };
    }

    return { success: true, pagination: { page, limit }, data: rows };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective" || LOG_MODE === "kafka") {
      reqLog.error("get messages by room error", { req_id: reqId, extra: { room_origin_id, error: err.message } });
    }
    set.status = 500;
    return { success: false, error: err.message };
  }
};