// Pruebas de carga de la API de VAIVÉN con k6 (spec 017, T010).
//
// Uso:
//   k6 run -e BASE_URL=https://host:8443 -e PROFILE=stress tests/load/api.js
//
// Variables:
//   BASE_URL  URL base de la API (obligatoria).
//   PROFILE   stress | soak | smoke (smoke por defecto).
//   TARGET    staging | local: elige la intensidad (staging por defecto, la
//             más suave). "local" es la API levantada dentro del runner.
//   DURATION  duración del perfil soak (p. ej. 15m); por defecto 10m en
//             staging y 15m en local.
//   P95_MS    umbral de p95 en ms para las respuestas esperadas; por defecto
//             800 en staging y 300 en local.
//
// La API limita a 120 peticiones por minuto por IP y responde 429
// rate_limited (Art. XIV.2). Ese 429 es una respuesta esperada: no cuenta en
// http_req_failed y se mide aparte en la métrica rate_limited. Lo que sí
// falla es cualquier otro código, un cuerpo sin el formato esperado o un 429
// sin el envoltorio del Art. IV.
import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || '').replace(/\/+$/, '');
const PROFILE = __ENV.PROFILE || 'smoke';
const TARGET = __ENV.TARGET || 'staging';

if (!BASE_URL) {
  throw new Error('Define BASE_URL, p. ej. -e BASE_URL=https://host:8443');
}
if (!['staging', 'local'].includes(TARGET)) {
  throw new Error(`TARGET desconocido: ${TARGET} (staging | local)`);
}

const rateLimited = new Counter('rate_limited');
const rateLimitedRate = new Rate('rate_limited_rate');
const badBody = new Rate('bad_body');

http.setResponseCallback(http.expectedStatuses(200, 429));

// Perfiles por destino. Staging es una VM de ~1 GiB con la API limitada a
// 160 MiB: nunca se le manda más de ~6 peticiones por segundo.
const profiles = {
  staging: {
    smoke: {
      executor: 'constant-arrival-rate',
      rate: 1, timeUnit: '1s', duration: '30s',
      preAllocatedVUs: 2, maxVUs: 5,
    },
    // Estrés: sube por encima del límite de 120/min para ver el 429 y que la
    // API siga respondiendo rápido mientras limita.
    stress: {
      executor: 'ramping-arrival-rate',
      startRate: 1, timeUnit: '1s',
      preAllocatedVUs: 5, maxVUs: 20,
      stages: [
        { target: 2, duration: '1m' },
        { target: 6, duration: '2m' },
        { target: 6, duration: '1m' },
        { target: 0, duration: '30s' },
      ],
    },
    // Fatiga: 1.5 peticiones/s = 90/min, por debajo del límite, sostenido.
    soak: {
      executor: 'constant-arrival-rate',
      rate: 3, timeUnit: '2s', duration: __ENV.DURATION || '10m',
      preAllocatedVUs: 3, maxVUs: 10,
    },
  },
  local: {
    smoke: {
      executor: 'constant-arrival-rate',
      rate: 5, timeUnit: '1s', duration: '30s',
      preAllocatedVUs: 5, maxVUs: 20,
    },
    stress: {
      executor: 'ramping-arrival-rate',
      startRate: 10, timeUnit: '1s',
      preAllocatedVUs: 50, maxVUs: 300,
      stages: [
        { target: 100, duration: '1m' },
        { target: 300, duration: '2m' },
        { target: 600, duration: '2m' },
        { target: 0, duration: '30s' },
      ],
    },
    soak: {
      executor: 'constant-arrival-rate',
      rate: 100, timeUnit: '1s', duration: __ENV.DURATION || '15m',
      preAllocatedVUs: 20, maxVUs: 100,
    },
  },
};

const scenario = profiles[TARGET][PROFILE];
if (!scenario) {
  throw new Error(`PROFILE desconocido: ${PROFILE} (smoke | stress | soak)`);
}

const p95 = Number(__ENV.P95_MS || (TARGET === 'local' ? 300 : 800));

const thresholds = {
  // Errores reales: todo lo que no sea 200 o 429.
  http_req_failed: ['rate<0.01'],
  // Latencia de las respuestas esperadas (200 y 429).
  'http_req_duration{expected_response:true}': [`p(95)<${p95}`],
  checks: ['rate>0.99'],
  bad_body: ['rate<0.01'],
};
// En smoke y soak de staging se va por debajo del límite: casi no debe haber 429.
if (TARGET === 'staging' && PROFILE !== 'stress') {
  thresholds.rate_limited_rate = ['rate<0.05'];
}

export const options = {
  scenarios: { [PROFILE]: scenario },
  thresholds,
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  // Sin -k: el certificado de staging debe ser válido.
  insecureSkipTLSVerify: false,
  userAgent: 'vaiven-load-test/k6',
};

const endpoints = [
  { name: 'healthz', path: '/healthz', ok: (b) => b.status === 'ok' },
  { name: 'meta', path: '/api/v1/meta', ok: (b) => Boolean(b.name && b.version && b.environment) },
];

function parse(res) {
  try {
    return res.json();
  } catch (_) {
    return null;
  }
}

export default function () {
  const ep = endpoints[Math.floor(Math.random() * endpoints.length)];
  const res = http.get(`${BASE_URL}${ep.path}`, { tags: { name: ep.name } });
  const body = parse(res);
  const limited = res.status === 429;

  rateLimited.add(limited ? 1 : 0);
  rateLimitedRate.add(limited);

  let bodyOk;
  if (limited) {
    // 429 con el envoltorio del Art. IV y Retry-After.
    bodyOk = Boolean(body && body.error && body.error.code === 'rate_limited' &&
      body.error.requestId && res.headers['Retry-After']);
  } else {
    bodyOk = res.status === 200 && Boolean(body) && ep.ok(body);
  }
  badBody.add(!bodyOk);

  check(res, {
    'status 200 o 429': (r) => r.status === 200 || r.status === 429,
    'cuerpo válido': () => bodyOk,
    'tiene X-Request-Id': (r) => Boolean(r.headers['X-Request-Id']),
  });
}
