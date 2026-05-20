import { Elysia } from "elysia";
import { getMessages, getMessagesByRoom } from "../controllers/message.controller";

export const messageRoutes = new Elysia()
  .get("/messages", getMessages)
  .get("/messages/room/:room_origin_id", getMessagesByRoom);

  