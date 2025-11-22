import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '1m', target: 5 },
    { duration: '3m', target: 5 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<300'],
    http_req_failed: ['rate<0.001'],
  },
};

const BASE_URL = 'http://localhost:8080';

export default function () {
  const endpoints = [
    '/team/get?team_name=backend',
    '/users/getReview?user_id=u1',
    '/stats/assignments',
    '/health',
  ];

  for (const endpoint of endpoints) {
    const res = http.get(`${BASE_URL}${endpoint}`);
    
    check(res, {
      [`${endpoint} status 200`]: (r) => r.status === 200,
      [`${endpoint} response time < 300ms`]: (r) => r.timings.duration < 300,
    });
    
    sleep(0.2);
  }
}