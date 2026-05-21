import { SQL } from "bun";

const db = new SQL(
  process.env.DATABASE_URL ||
    "postgresql://postgres:postgres@localhost:5433/logging_benchmark",
  {
    max: 50,
    idleTimeout: 20,
    connectionTimeout: 10,
  },
);

try {
  await db`SET statement_timeout TO '5s'`;
  console.log("statement_timeout configured");
} catch (err) {
  console.error("failed to configure statement_timeout", err);
}

export default db;