import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '1m',  target: 200 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<50'],
  },
};

export default function () {
  const res = http.get('http://host.docker.internal:8080/'); 
  
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
  
  sleep(0.01);
}



/* 
MSYS_NO_PATHCONV=1 docker run --rm -it \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)"://scripts \
  -e K6_WEB_DASHBOARD=true \
  -e K6_WEB_DASHBOARD_HOST=0.0.0.0 \
  -p 5665:5665 \
  grafana/k6 run //scripts/loadtest.js
  */