# Spec 014 — Sugerencias con IA al publicar (backend)

- **Dueño:** BE-A (con BE-B en revisión) · **Prioridad:** P1 · **Ventana:** 23 nov–1 dic · **Depende de:** 004, 005, 011 (filtro), 013
- **Constitución:** v1.4.0 · **Estado:** Borrador

## 1. Objetivo y por qué
La entrega oficial (1–4 dic) exige que todos los specs estén completos, incluida la función de IA que menciona el documento de idea: **sugerir título y categoría a partir de una fotografía, siempre con revisión de quien publica**. La IA nunca decide: solo propone, y la persona acepta, edita o ignora. Se construye **después** de congelar la 1.0 (22 nov) y sale como actualización v1.1, para no frenar la revisión de Google Play.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona que publica, quiero que la app sugiera un título y una categoría a partir de mi primera foto, para publicar más rápido. |
| H-02 | P1 | Como persona que publica, quiero poder editar o descartar la sugerencia, para decidir yo lo que se publica. |
| H-03 | P1 | Como equipo, queremos apagar la función con una variable de entorno, para que un fallo del proveedor no afecte la publicación normal. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `POST /api/v1/ai/listing-suggestions` (autenticado) con `{ "imagePublicId": "…" }` devuelve `{ "title": string, "category": <clave POL-14>, "description"?: string }`. |
| RF-02 | El backend **no acepta URL**: recibe el `publicId` de una imagen con finalidad `item_photo` **subida por esa misma persona** (`images.Service`) y construye él mismo la URL de la miniatura. Evita SSRF y uso de imágenes ajenas. |
| RF-03 | **Proveedor de IA con capa gratuita y visión** detrás de la interfaz `suggest.Suggester`. Provisional: Gemini API (modelo con visión, capa gratuita), llamado por HTTP con `net/http` y sin SDK; la clave solo en `AI_API_KEY` (variable de entorno del backend). Cambiar de proveedor toca un solo adaptador. |
| RF-04 | La salida del modelo se **restringe**: `category` debe ser una de las claves de POL-14 (si no, se omite); `title` se recorta a 80 caracteres; todo texto pasa por `moderation.Checker` (spec 011) y se descarta si no es permitido. |
| RF-05 | **Límite de uso** por persona (provisional 20 sugerencias por día, `AI_DAILY_LIMIT`) y global del proveedor; al excederse, `rate_limited`. Contador en MongoDB con índice TTL. |
| RF-06 | **Interruptor** `AI_ENABLED`. `GET /api/v1/meta` expone `features.aiSuggestions` (bool). Apagada o con el proveedor caído (tiempo de espera de 10 s), responde `service_unavailable` y la app sigue funcionando sin sugerencias. |
| RF-07 | **Privacidad:** la foto se envía al proveedor solo a petición de la persona; no se guardan imágenes ni respuestas del modelo salvo el contador de uso; los registros no incluyen la imagen ni el texto sugerido. El uso queda en el aviso de privacidad y en «Seguridad de datos» de Play. |
| RF-08 | Contrato en `api/paths/ai.yaml`; errores con el envoltorio estándar. |
| RNF-01 | p95 < 6 s incluyendo al proveedor; la prueba usa un proveedor falso (sin red). |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una foto propia, entonces devuelve título y una categoría válida de POL-14. |
| CA-02 | Dado un `publicId` de otra persona o de evidencia, entonces `403`/`404` y no se llama al proveedor. |
| CA-03 | Dado un título que el filtro rechaza, entonces no se devuelve. |
| CA-04 | Dado el proveedor caído o `AI_ENABLED=false`, entonces `service_unavailable` y `meta` lo indica. |
| CA-05 | Dado el límite diario superado, entonces `rate_limited`. |
| CA-06 | Dado el repositorio, entonces no hay clave del proveedor. |

## 5. Fuera de alcance
Entrenar modelos, descripciones largas generadas, IA en el chat, sugerencia de precio por IA (ya existe la fórmula de la spec 005), moderación de imágenes por IA.

## 6. Preguntas abiertas
- Resuelto (28 oct): el proveedor es **Gemini** (capa gratuita; proyecto escolar). Como la capa gratuita puede usar los datos enviados para mejorar el servicio, el aviso de privacidad y «Seguridad de datos» de Play deben decirlo claramente (mobile 014-T006).
- [NEEDS CLARIFICATION: ¿Qué se pide exactamente para la «entrega académica» (solo sugerencia por foto, o también ML propio)? Provisional: solo sugerencia por foto.]
