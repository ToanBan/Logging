import { FastifyInstance } from "fastify";
import { getMessages, getMessagesByRoom } from "../controllers/message.controller";

export default async function messageRoutes(fastify: FastifyInstance) {
  fastify.get("/messages", getMessages);
  fastify.get("/messages/room/:room_origin_id", getMessagesByRoom);
}
