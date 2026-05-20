import { FastifyRequest, FastifyReply } from "fastify";
import sql from "../config/db";

const LOG_MODE = (process.env.LOG_MODE || "none").trim().toLowerCase();

interface GetMessagesQuery {
  page?: string;
  limit?: string;
  room_origin_id?: string;
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
  if (LOG_MODE === "structured") {
    req.log.info({ event: "get_messages_request", query: req.query });
  }

  const page = Math.max(1, parseInt(req.query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit || "10", 10)));
  const offset = (page - 1) * limit;
  const roomOriginId = req.query.room_origin_id;

  try {
    let rows

    if (roomOriginId) {
      rows = await sql`
        SELECT * FROM chat_message
        WHERE room_origin_id = ${roomOriginId}
        ORDER BY created_at DESC
        LIMIT ${limit} OFFSET ${offset}
      `
    } else {
      rows = await sql`
        SELECT * FROM chat_message
        ORDER BY created_at DESC
        LIMIT ${limit} OFFSET ${offset}
      `
    }

    return {
      success: true,
      pagination: { page, limit },
      data: rows,
    };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective") {
      req.log.error({ event: "get_messages_error", error: err.message });
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
  const { room_origin_id } = req.params;

  if (LOG_MODE === "structured") {
    req.log.info({ event: "get_messages_by_room_request", room_origin_id });
  }

  const page = Math.max(1, parseInt(req.query.page || "1", 10));
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit || "10", 10)));
  const offset = (page - 1) * limit;

  try {
    const rows = await sql`
      SELECT * FROM chat_message
      WHERE room_origin_id = ${room_origin_id}
      ORDER BY created_at DESC
      LIMIT ${limit} OFFSET ${offset}
    `

    if (rows.length === 0) {
      if (LOG_MODE === "structured" || LOG_MODE === "selective") {
        req.log.warn({
          event: "get_messages_by_room_not_found",
          room_origin_id,
        });
      }
      reply
        .header("connection", "keep-alive")
        .status(404);
      return {
        success: false,
        error: `No messages found for room ${room_origin_id}`,
      };
    }

    return {
      success: true,
      pagination: { page, limit },
      data: rows,
    };
  } catch (err: any) {
    if (LOG_MODE === "structured" || LOG_MODE === "selective") {
      req.log.error({
        event: "get_messages_by_room_error",
        room_origin_id,
        error: err.message,
      });
    }
    reply.status(500);
    return { success: false, error: err.message };
  }
};