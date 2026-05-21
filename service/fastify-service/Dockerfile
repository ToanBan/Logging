FROM node:22-alpine AS base
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable

WORKDIR /app

COPY service/fastify-service/package.json ./
RUN pnpm install --frozen-lockfile --ignore-scripts 2>/dev/null || pnpm install --ignore-scripts

COPY service/fastify-service/tsconfig.json ./
COPY service/fastify-service/src ./src
COPY shared /shared

EXPOSE 3001

CMD ["pnpm", "exec", "tsx", "src/index.ts"]
