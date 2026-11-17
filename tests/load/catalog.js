// Rendimiento de lectura del catálogo y de las reservas (spec 012, T006, RF-08,
// CA-07). Complementa api.js (spec 017), que solo mide /healthz y /meta.
//
// SOLO en local, nunca contra staging: el setup crea solicitudes de reserva.
// Datos: `go run ./cmd/admin seed --scale 100` en una base local.
//
// Uso (reproducible; ver docs/rendimiento.md):
//   k6 run -e BASE_URL=http://localhost:8080 \
//     -e REQUESTER_EMAIL=... -e REQUESTER_PASSWORD=... \
//     -e OWNER_EMAIL=...     -e OWNER_PASSWORD=... tests/load/catalog.js
//
// Variables:
//   BASE_URL            API local (obligatoria; se rechaza cualquier host que
//                       no sea localhost, 127.0.0.1 o host.containers.internal).
//   REQUESTER_*/OWNER_* cuentas del seed (del archivo de SEED_CREDENTIALS_FILE).
//   DURATION            duración de la medición (5m por defecto).
//   BOOKINGS            solicitudes que el setup crea para REQUESTER (20).
//   P95_MS              umbral de p95 (500 ms, RF-08).
//
// Ritmo: 8 iteraciones cada 5 s. Como la búsqueda con segunda página hace dos
// peticiones (10 % de las iteraciones), son ~1,76 peticiones/s (~106/min),
// por debajo del límite de 120/min por IP (Art. XIV.2): se miden respuestas
// 200, no 429. Con 9 cada 5 s se llegó a ~122/min y aparecieron 429.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || '').replace(/\/+$/, '');
const P95 = Number(__ENV.P95_MS || 500);
const BOOKINGS = Number(__ENV.BOOKINGS || 20);

if (!/^https?:\/\/(localhost|127\.0\.0\.1|host\.containers\.internal)(:\d+)?$/.test(BASE_URL)) {
  throw new Error('BASE_URL debe ser una API local (esta prueba crea datos)');
}
for (const v of ['REQUESTER_EMAIL', 'REQUESTER_PASSWORD', 'OWNER_EMAIL', 'OWNER_PASSWORD']) {
  if (!__ENV[v]) throw new Error(`Falta ${v}`);
}

const limited = new Rate('rate_limited_rate');

export const options = {
  scenarios: {
    lectura: {
      executor: 'constant-arrival-rate',
      rate: 8, timeUnit: '5s', duration: __ENV.DURATION || '5m',
      preAllocatedVUs: 4, maxVUs: 20,
    },
  },
  // El setup crea BOOKINGS solicitudes y espera un minuto para vaciar la
  // ventana del límite de tasa antes de medir.
  setupTimeout: '5m',
  thresholds: {
    'http_req_duration{kind:search}': [`p(95)<${P95}`],
    'http_req_duration{kind:my_bookings}': [`p(95)<${P95}`],
    // Submétricas de conteo para el resumen.
    'http_reqs{kind:search}': ['count>0'],
    'http_reqs{kind:my_bookings}': ['count>0'],
    'http_req_failed{phase:measure}': ['rate<0.01'],
    'rate_limited_rate': ['rate<0.01'],
    checks: ['rate>0.99'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  userAgent: 'vaiven-load-test/k6-catalog',
};

function login(email, password) {
  const res = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' }, tags: { phase: 'setup' } });
  if (res.status !== 200) throw new Error(`login ${email}: ${res.status}`);
  return res.json('tokens.accessToken');
}

function day(offset) {
  const d = new Date(Date.now() + offset * 86400000);
  return d.toISOString().slice(0, 10);
}

export function setup() {
  const requester = login(__ENV.REQUESTER_EMAIL, __ENV.REQUESTER_PASSWORD);
  const owner = login(__ENV.OWNER_EMAIL, __ENV.OWNER_PASSWORD);
  const auth = (t) => ({ headers: { Authorization: `Bearer ${t}`, 'Content-Type': 'application/json' }, tags: { phase: 'setup' } });

  // Solicitudes de REQUESTER sobre objetos de otras personas, para que
  // GET /me/bookings tenga varias (representativo, CA-07).
  const page = http.get(`${BASE_URL}/api/v1/items?limit=50`, auth(requester));
  const cards = (page.json('items') || []).slice(0, BOOKINGS);
  const start = day(40), end = day(41);
  let created = 0;
  for (const c of cards) {
    const mode = c.modality === 'loan' ? 'none' : 'full_on_delivery';
    const q = http.get(`${BASE_URL}/api/v1/items/${c.id}/quote?startDate=${start}&endDate=${end}&paymentMode=${mode}`, auth(requester));
    if (q.status !== 200) continue;
    const b = http.post(`${BASE_URL}/api/v1/bookings`, JSON.stringify({
      itemId: c.id, startDate: start, endDate: end, paymentMode: mode, acceptedTotalCents: q.json('totalCents'),
    }), auth(requester));
    if (b.status === 201) created++;
  }
  sleep(65);
  return { requester, owner, created };
}

const categories = ['herramientas_manuales', 'herramientas_electricas', 'construccion_jardineria', 'movilidad', 'deportes', 'hogar', 'otros'];
const municipalities = ['Guadalajara', 'Zapopan', 'Tlaquepaque', 'Tonalá'];
const words = ['taladro', 'bicicleta', 'mesa', 'herramientas', 'escalera', 'proyector'];
const pick = (a) => a[Math.floor(Math.random() * a.length)];

// Mezcla de lecturas (pesos aproximados): búsquedas 70 %, reservas 30 %.
const requests = [
  { w: 20, kind: 'search', name: 'search_recent', path: () => '/api/v1/items?limit=20' },
  { w: 10, kind: 'search', name: 'search_recent_page2', path: () => '/api/v1/items?limit=20', page2: true },
  { w: 12, kind: 'search', name: 'search_category', path: () => `/api/v1/items?category=${pick(categories)}&limit=20` },
  { w: 10, kind: 'search', name: 'search_filters', path: () => `/api/v1/items?category=${pick(categories)}&modality=rent&municipality=${encodeURIComponent(pick(municipalities))}&limit=20` },
  { w: 10, kind: 'search', name: 'search_text', path: () => `/api/v1/items?q=${pick(words)}&limit=20` },
  { w: 8, kind: 'search', name: 'search_dates', path: () => `/api/v1/items?from=${day(20)}&to=${day(22)}&limit=20` },
  { w: 18, kind: 'my_bookings', name: 'my_bookings_requester', path: () => '/api/v1/me/bookings?role=requester&limit=20', as: 'requester' },
  { w: 12, kind: 'my_bookings', name: 'my_bookings_owner', path: () => '/api/v1/me/bookings?role=owner&limit=20', as: 'owner' },
];
const total = requests.reduce((s, r) => s + r.w, 0);

function choose() {
  let x = Math.random() * total;
  for (const r of requests) {
    if ((x -= r.w) < 0) return r;
  }
  return requests[0];
}

function get(path, token, r) {
  const res = http.get(`${BASE_URL}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
    tags: { kind: r.kind, name: r.name, phase: 'measure' },
  });
  limited.add(res.status === 429);
  check(res, {
    'status 200': (x) => x.status === 200,
    'tiene items': (x) => Array.isArray(x.json('items')),
  });
  return res;
}

export default function (data) {
  const r = choose();
  const token = r.as === 'owner' ? data.owner : data.requester;
  const res = get(r.path(), token, r);
  if (r.page2 && res.status === 200) {
    const next = res.json('nextCursor');
    if (next) get(`${r.path()}&cursor=${encodeURIComponent(next)}`, token, r);
  }
}

// Resumen: JSON completo en SUMMARY_FILE y una tabla corta por tipo.
export function handleSummary(data) {
  const row = (label, key) => {
    const m = data.metrics[key];
    if (!m) return `${label}: sin datos\n`;
    const v = m.values;
    const f = (x) => (x === undefined ? '-' : x.toFixed(1));
    return `${label}: n=${data.metrics[key.replace('http_req_duration', 'http_reqs')] ? data.metrics[key.replace('http_req_duration', 'http_reqs')].values.count : '?'} ` +
      `p50=${f(v.med)} p95=${f(v['p(95)'])} p99=${f(v['p(99)'])} max=${f(v.max)} ms\n`;
  };
  const text = '\n== RF-08 (ms) ==\n' +
    row('búsqueda    ', 'http_req_duration{kind:search}') +
    row('mis reservas', 'http_req_duration{kind:my_bookings}') +
    `solicitudes creadas en el setup: ${data.setup_data ? data.setup_data.created : '?'}\n`;
  return { stdout: text, [__ENV.SUMMARY_FILE || 'catalog-summary.json']: JSON.stringify(data, null, 2) };
}
