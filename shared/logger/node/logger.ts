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

async function connectProducer() {
  if (!producerConnected) {
    await producer.connect();

    const admin = kafka.admin();
    await admin.connect();
    await admin.createTopics({
      topics: [{ topic: "req-logs", numPartitions: 1, replicationFactor: 1 }],
      waitForLeaders: true,
    });
    await admin.disconnect();

    producerConnected = true;
  }
}

async function flushToKafka(entries: LogEntry[]) {
  try {
    await connectProducer();
    await producer.send({
      topic: "req-logs",
      messages: [{ value: JSON.stringify(entries) }],
    });
  } catch (err) {
    process.stderr.write(`[kafka-flush-error] ${JSON.stringify(err)}\n`);
  }
}

const BUFFER_SIZE = 64;

class RingBuffer {
  private buffer: LogEntry[] = [];

  push(entry: LogEntry) {
    if (this.buffer.length >= BUFFER_SIZE) {
      this.buffer.shift();
    }
    this.buffer.push(entry);
  }

  flush(): LogEntry[] {
    return [...this.buffer];
  }

  clear() {
    this.buffer = [];
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
  const ring = new RingBuffer();

  return {
    debug(msg: string, ctx: LogContext = {}) {
      ring.push(buildEntry(service, "debug", msg, { ...ctx, req_id: reqId }));
    },
    info(msg: string, ctx: LogContext = {}) {
      ring.push(buildEntry(service, "info", msg, { ...ctx, req_id: reqId }));
    },
    warn(msg: string, ctx: LogContext = {}) {
      ring.push(buildEntry(service, "warn", msg, { ...ctx, req_id: reqId }));
      flushToKafka(ring.flush());
      ring.clear();
    },
    error(msg: string, ctx: LogContext = {}) {
      ring.push(buildEntry(service, "error", msg, { ...ctx, req_id: reqId }));
      flushToKafka(ring.flush());
      ring.clear();
    },
    fatal(msg: string, ctx: LogContext = {}) {
      ring.push(buildEntry(service, "fatal", msg, { ...ctx, req_id: reqId }));
      flushToKafka(ring.flush());
      ring.clear();
    },
    done() {
      ring.clear();
    },
  };
}

export { createLogger, createRequestLogger };
export type { LogContext };