FROM node:22-alpine AS base
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable

WORKDIR /app

COPY package.json ./
RUN pnpm install --frozen-lockfile --ignore-scripts 2>/dev/null || pnpm install --ignore-scripts

COPY tsconfig.json ./
COPY src ./src

EXPOSE 3001

CMD ["pnpm", "exec", "tsx", "src/index.ts"]
