Dự án của tôi là một monorepo benchmark logging với cấu trúc như sau, hãy đọc toàn bộ cấu trúc thư mục và code hiện tại, sau đó tự sinh ra Dockerfile cho từng service và docker-compose.yml ở root.
Yêu cầu:
Dockerfile từng service:

Đọc code và package manager của từng service để viết Dockerfile phù hợp
Fastify, Elysia: dùng pnpm, TypeScript, chạy bằng tsx
Elysia: dùng Bun runtime
Gin, Fiber: Go, build binary rồi chạy
FastAPI: Python, chạy bằng uvicorn
Django: Python, chạy bằng manage.py runserver

docker-compose.yml:

Đặt ở root project
LOG_MODE dùng biến môi trường bên ngoài: ${LOG_MODE:-none}
DATABASE_URL trỏ về host.docker.internal với port và credentials đọc từ file .env hiện tại của từng service
Map đúng port từng service
Không dùng volume, không dùng network phức tạp

Cách chạy sau khi xong:
LOG_MODE=none docker compose up --build
LOG_MODE=structured docker compose up
LOG_MODE=selective docker compose up