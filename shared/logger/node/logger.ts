import pino from "pino";

interface LogContext {
  req_id?: string;
  trace_id?: string;
  user_id?: string;
  extra?: Record<string, unknown>;
}

const _pino = pino({
  base: undefined,
  timestamp: () => `,"timestamp":"${new Date().toISOString()}"`,
  formatters: {
    level: (label) => ({ level: label }),
  },
});

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

export { createLogger };
export type { LogContext };