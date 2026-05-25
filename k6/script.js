import http from "k6/http";
import { sleep } from "k6";
import { Trend, Counter } from "k6/metrics";

const durationAllMessages = new Trend("duration_all_messages", true);
const durationRoomFound = new Trend("duration_room_found", true);
const durationRoomNotFound = new Trend("duration_room_not_found", true);
const errorCount = new Counter("error_count");

const BASE_URL = "http://localhost:3004";
const ROOM_FOUND = "38224506666472305314";
const ROOM_NOT_FOUND = "room-not-exist-999";

export const options = {
  scenarios: {
    all_messages: {
      executor: "constant-vus",
      vus: 22,
      duration: "60s",
      exec: "testAllMessages",
    },
    room_found: {
      executor: "constant-vus",
      vus: 22,
      duration: "60s",
      exec: "testRoomFound",
    },
    room_not_found: {
      executor: "constant-vus",
      vus: 6,
      duration: "60s",
      exec: "testRoomNotFound",
    },
  },
  thresholds: {
    duration_all_messages: ["p(95)<2000", "p(99)<3000"],
    duration_room_found: ["p(95)<2000", "p(99)<3000"],
    duration_room_not_found: ["p(95)<2000", "p(99)<3000"],
  },
};

export function testAllMessages() {
  const r = http.get(`${BASE_URL}/messages`);
  if (r.status !== 200) errorCount.add(1);
  durationAllMessages.add(r.timings.duration);
  sleep(0.1);
}

export function testRoomFound() {
  const r = http.get(`${BASE_URL}/messages/room/${ROOM_FOUND}`);
  if (r.status !== 200) errorCount.add(1);
  durationRoomFound.add(r.timings.duration);
  sleep(0.1);
}

export function testRoomNotFound() {
  const r = http.get(`${BASE_URL}/messages/room/${ROOM_NOT_FOUND}`);
  if (r.status !== 404) errorCount.add(1);
  durationRoomNotFound.add(r.timings.duration);
  sleep(0.1);
}