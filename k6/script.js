import http from "k6/http";
import { sleep } from "k6";
import { Trend } from "k6/metrics";
import { vu } from "k6/execution";

const durationAllMessages = new Trend("duration_all_messages", true);
const durationRoomMessages = new Trend("duration_room_messages", true);
const BASE_URL = "http://localhost:3005";
export const options = {
  scenarios: {
    logging_benchmark: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "15s", target: 50 },
        { duration: "30s", target: 50 },
        { duration: "15s", target: 0 },
      ],
      gracefulStop: "15s",
    },
  },
  thresholds: {
    duration_all_messages: ["p(95)<2000"],
    duration_room_messages: ["p(95)<2000"],
  },
};

export default function () {
  const vuId = vu.idInTest;
  if (vuId % 2 !== 0) {
    const r1 = http.get(`${BASE_URL}/messages`);
    durationAllMessages.add(r1.timings.duration);
  } else {
    const r2 = http.get(`${BASE_URL}/messages/room/1`);
    durationRoomMessages.add(r2.timings.duration);
  }

  sleep(0.1);
}
