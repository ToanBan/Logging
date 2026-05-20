import postgres from "postgres";

const sql = postgres(
  process.env.DATABASE_URL ||
    "postgresql://postgres:postgres@localhost:5433/logging_benchmark",
  {
    max: 50,
    idle_timeout: 20,
    connect_timeout: 10,
    connection: {
      statement_timeout: 5000, // ← thêm vào đây
    },
  },
);

export default sql;
