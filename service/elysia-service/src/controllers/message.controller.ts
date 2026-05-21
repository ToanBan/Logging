import db from "../config/db";
import { createLogger } from "@shared/logger";

const LOG_MODE = (process.env.LOG_MODE || "none")
  .trim()
  .toLowerCase();

const log = createLogger("elysia-service");

export const getMessages = async ({
  query,
  store,
  set,
}: any) => {
  const reqId = store.reqId as string;

  const tStart = performance.now();

  if (LOG_MODE === "structured") {
    log.info("get messages request", {
      req_id: reqId,
      extra: { query },
    });
  }

  const page = Math.max(
    1,
    parseInt(query.page || "1", 10),
  );

  const limit = Math.max(
    1,
    Math.min(100, parseInt(query.limit || "10", 10)),
  );

  const offset = (page - 1) * limit;

  const tParamDone = performance.now();

  try {
    const rows = await db`
      SELECT *
      FROM chat_message
      ORDER BY created_at DESC
      LIMIT ${limit}
      OFFSET ${offset}
    `;

    const tDbDone = performance.now();

    const pParams = tParamDone - tStart;
    const pDb = tDbDone - tParamDone;
    const pTotal = performance.now() - tStart;

    log.info(
      `[ELYSIA_PERF_GET_ALL] Mode: ${LOG_MODE.toUpperCase()} | ParseParam: ${pParams.toFixed(3)}ms | DB_Query: ${pDb.toFixed(3)}ms | Total: ${pTotal.toFixed(3)}ms`,
      {
        req_id: reqId,
      },
    );

    return {
      success: true,
      pagination: {
        page,
        limit,
      },
      data: rows,
    };
  } catch (err: any) {
    if (
      LOG_MODE === "structured" ||
      LOG_MODE === "selective"
    ) {
      log.error("get messages error", {
        req_id: reqId,
        extra: {
          error: err.message,
        },
      });
    }

    set.status = 500;

    return {
      success: false,
      error: err.message,
    };
  }
};

export const getMessagesByRoom = async ({
  params,
  query,
  store,
  set,
}: any) => {
  const reqId = store.reqId as string;

  const tStart = performance.now();

  const { room_origin_id } = params;

  if (LOG_MODE === "structured") {
    log.info("get messages by room request", {
      req_id: reqId,
      extra: {
        room_origin_id,
      },
    });
  }

  const page = Math.max(
    1,
    parseInt(query.page || "1", 10),
  );

  const limit = Math.max(
    1,
    Math.min(100, parseInt(query.limit || "10", 10)),
  );

  const offset = (page - 1) * limit;

  const tParamDone = performance.now();

  try {
    const rows = await db`
      SELECT *
      FROM chat_message
      WHERE room_origin_id = ${room_origin_id}
      ORDER BY created_at DESC
      LIMIT ${limit}
      OFFSET ${offset}
    `;

    const tDbDone = performance.now();

    const pParams = tParamDone - tStart;
    const pDb = tDbDone - tParamDone;

    if (rows.length === 0) {
      if (
        LOG_MODE === "structured" ||
        LOG_MODE === "selective"
      ) {
        log.warn("room not found", {
          req_id: reqId,
          extra: {
            room_origin_id,
          },
        });
      }

      set.status = 404;

      set.headers = {
        connection: "keep-alive",
      };

      const pTotal = performance.now() - tStart;

      log.info(
        `[ELYSIA_PERF_BY_ROOM_404] Mode: ${LOG_MODE.toUpperCase()} | Room: ${room_origin_id} | ParseParam: ${pParams.toFixed(3)}ms | DB_Query: ${pDb.toFixed(3)}ms | Total: ${pTotal.toFixed(3)}ms`,
        {
          req_id: reqId,
        },
      );

      return {
        success: false,
        error: `No messages found for room ${room_origin_id}`,
      };
    }

    const pTotal = performance.now() - tStart;

    log.info(
      `[ELYSIA_PERF_BY_ROOM_200] Mode: ${LOG_MODE.toUpperCase()} | Room: ${room_origin_id} | ParseParam: ${pParams.toFixed(3)}ms | DB_Query: ${pDb.toFixed(3)}ms | Total: ${pTotal.toFixed(3)}ms`,
      {
        req_id: reqId,
      },
    );

    return {
      success: true,
      pagination: {
        page,
        limit,
      },
      data: rows,
    };
  } catch (err: any) {
    if (
      LOG_MODE === "structured" ||
      LOG_MODE === "selective"
    ) {
      log.error("get messages by room error", {
        req_id: reqId,
        extra: {
          room_origin_id,
          error: err.message,
        },
      });
    }

    set.status = 500;

    return {
      success: false,
      error: err.message,
    };
  }
};