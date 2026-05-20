FROM oven/bun:1 AS base

WORKDIR /app

COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

COPY tsconfig.json ./
COPY index.ts ./
COPY src ./src

EXPOSE 3002

CMD ["bun", "run", "index.ts"]
