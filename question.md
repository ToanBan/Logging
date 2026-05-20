bạn hãy đọc dự án của tôi, tôi đang gặp lỗi tại sao tôi truyền LOG_MODE="selective"
docker compose down
$env:LOG_MODE="selective"
docker compose up

khi tôi ost":"localhost:3001","remoteAddress":"172.19.0.1","remotePort":36474},"msg":"incoming request"}
fastify-service   | {"level":30,"time":1779255409424,"pid":35,"hostname":"36660c21ef42","reqId":"req-iz","res":{"statusCode":200},"responseTime":2.4811439998447895,"msg":"request completed"}
fastify-service   | {"level":30,"time":1779255409427,"pid":35,"hostname":"36660c21ef42","reqId":"req-iy","res":{"statusCode":200},"responseTime":5.486574999988079,"msg":"request completed"}
fastify-service   | {"level":30,"time":1779255409428,"pid":35,"hostname":"36660c21ef42","reqId":"req-j0","res":{"statusCode":200},"responseTime":5.7646019998937845,"msg":"request completed"}
fastify-service   | {"level":30,"time":1779255409428,"pid":35,"hostname":"36660c21ef42","reqId":"req-j1","req":{"method":"GET","url":"/messages","host":"localhost:3001","remoteAddress":"172.19.0.1",
nó vẫn trả về mấy cái này, mấy cái này selective làm gì có
hãy fix lại giúp tôi, thằng selective thì nó chỉ check những chỗ error và warn thôi chứ