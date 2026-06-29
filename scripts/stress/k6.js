import http from 'k6/http';
import { check } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
    scenarios: {
        constant_request_rate: {
            executor: 'constant-arrival-rate',
            rate: 2000,           // 2000 requests per second
            timeUnit: '1s',
            duration: '30s',      // for 30 seconds
            preAllocatedVUs: 100, // pre-allocate 100 VUs
            maxVUs: 1000,         // up to 1000 VUs if needed
        },
    },
};

const payload = JSON.stringify({
    type: 'stress.test.k6',
    payload: {
        amount: 250,
        currency: 'USD',
        description: 'K6 load test payload'
    }
});

const params = {
    headers: {
        'Content-Type': 'application/json',
        'X-API-Key': 'kestrel-dev-key',
    },
};

export default function () {
    // Note: use host.docker.internal to reach the host's port 8080 from inside the k6 docker container
    const res = http.post('http://host.docker.internal:8080/api/events', payload, params);
    
    check(res, {
        'is status 202': (r) => r.status === 202,
    });
}
