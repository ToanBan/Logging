FROM oven/bun:1 AS base

WORKDIR /app

COPY service/elysia-service/package.json service/elysia-service/bun.lock* ./
RUN bun install --frozen-lockfile

COPY service/elysia-service/tsconfig.json ./
COPY service/elysia-service/index.ts ./
COPY service/elysia-service/src ./src
COPY shared /shared

EXPOSE 3002

CMD ["bun", "run", "index.ts"]
