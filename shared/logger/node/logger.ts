import pino from "pino";
import { Kafka } from "kafkajs";
import path from "path";
import * as fs from "fs";

interface LogContext {
  req_id?: string;
  trace_id?: string;
  user_id?: string;
  extra?: Record<string, unknown>;
}

interface LogEntry {
  timestamp: string;
  level: string;
  service: string;
  req_id?: string;
  trace_id?: string;
  user_id?: string;
  msg: string;
  extra?: Record<string, unknown>;
}

const LOGS_DIRECTORY = path.join("/shared", "logger", "logs");
const COMBINED_LOG_PATH = path.join(LOGS_DIRECTORY, "combined.log");

if (!fs.existsSync(LOGS_DIRECTORY)) {
  fs.mkdirSync(LOGS_DIRECTORY, { recursive: true });
}

const logStream = fs.createWriteStream(COMBINED_LOG_PATH, { flags: "a" });

const _pino = pino(
  {
    base: undefined,
    timestamp: () => `,"timestamp":"${new Date().toISOString()}"`,
    formatters: {
      level: (label) => ({ level: label }),
    },
  },
  logStream
);

const kafka = new Kafka({
  clientId: "logger",
  brokers: [process.env.KAFKA_BROKER || "kafka:9092"],
  logLevel: 0,
});
const producer = kafka.producer();
let producerConnected = false;
let connectingPromise: Promise<void> | null = null;

async function connectProducer() {
  if (producerConnected) return;
  if (!connectingPromise) {
    connectingPromise = (async () => {
      await producer.connect();

      const admin = kafka.admin();
      await admin.connect();
      await admin.createTopics({
        topics: [{ topic: "req-logs", numPartitions: 1, replicationFactor: 1 }],
        waitForLeaders: true,
      });
      await admin.disconnect();

      producerConnected = true;
    })();
  }
  await connectingPromise;
}

async function flushToKafka(entries: LogEntry[]) {
  try {
    await connectProducer();
    await producer.send({
      topic: "req-logs",
      messages: [{ value: JSON.stringify(entries) }],
    });
  } catch (err) {
    // fallback: ghi ra file nếu kafka fail
    entries.forEach(e => _pino.error(e));
    process.stderr.write(`[kafka-flush-error] ${JSON.stringify(err)}\n`);
  }
}

function buildLog(service: string, msg: string, ctx: LogContext) {
  return {
    service,
    req_id: ctx.req_id,
    trace_id: ctx.trace_id,
    user_id: ctx.user_id,
    msg,
    extra: ctx.extra,
  };
}

function buildEntry(service: string, level: string, msg: string, ctx: LogContext): LogEntry {
  return {
    timestamp: new Date().toISOString(),
    level,
    service,
    req_id: ctx.req_id,
    trace_id: ctx.trace_id,
    user_id: ctx.user_id,
    msg,
    extra: ctx.extra,
  };
}

function createLogger(service: string) {
  return {
    debug(msg: string, ctx: LogContext = {}) {
      _pino.debug(buildLog(service, msg, ctx));
    },
    info(msg: string, ctx: LogContext = {}) {
      _pino.info(buildLog(service, msg, ctx));
    },
    warn(msg: string, ctx: LogContext = {}) {
      _pino.warn(buildLog(service, msg, ctx));
    },
    error(msg: string, ctx: LogContext = {}) {
      _pino.error(buildLog(service, msg, ctx));
    },
    fatal(msg: string, ctx: LogContext = {}) {
      _pino.fatal(buildLog(service, msg, ctx));
    },
  };
}

function createRequestLogger(service: string, reqId: string) {
  return {
    debug(msg: string, ctx: LogContext = {}) {
      flushToKafka([buildEntry(service, "debug", msg, { ...ctx, req_id: reqId })]);
    },
    info(msg: string, ctx: LogContext = {}) {
      flushToKafka([buildEntry(service, "info", msg, { ...ctx, req_id: reqId })]);
    },
    warn(msg: string, ctx: LogContext = {}) {
      flushToKafka([buildEntry(service, "warn", msg, { ...ctx, req_id: reqId })]);
    },
    error(msg: string, ctx: LogContext = {}) {
      flushToKafka([buildEntry(service, "error", msg, { ...ctx, req_id: reqId })]);
    },
    fatal(msg: string, ctx: LogContext = {}) {
      flushToKafka([buildEntry(service, "fatal", msg, { ...ctx, req_id: reqId })]);
    },
  };
}

export { createLogger, createRequestLogger };
export type { LogContext };