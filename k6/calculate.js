import http from 'k6/http';

export const options = {
  scenarios: {
    exact_10_requests: {
      executor: 'per-vu-iterations',
      vus: 10,                 
      iterations: 1,          
      maxDuration: '10s',            
    },
  },
};

export default function () {
  const url = 'http://localhost:3001/messages/room/1'; 
  http.get(url); 
}