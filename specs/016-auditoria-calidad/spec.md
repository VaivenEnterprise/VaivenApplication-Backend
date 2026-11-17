# Spec 016 — Auditoría de calidad y consolidación (backend)

- **Dueño:** BE-B (coordina) con BE-A (corrige lo de sus specs) · **Prioridad:** P0 para la entrega · **Ventana:** 26 nov–1 dic · **Depende de:** 001–015 y 017 (las audita); empieza cuando la 015 termine su despliegue verificado (Art. XIII.5)
- **Constitución:** v2.0.0 · **Estado:** Borrador (2026-11-17), pendiente de aprobación (Art. I y X.5)
- **Antecedente:** la antigua spec 016 (siete categorías de pruebas y pipeline DevSecOps, D-113) se eliminó el 2026-11-01 (D-115). El pipeline quedó en la 017 y la integración en la 012. Esta 016 es **nueva**: reutiliza el número para cerrar lo que ninguna spec cubre y no repite lo que ya hacen la 012 y la 017.

## 1. Objetivo y por qué
Al terminar la 015 habrá 16 specs de backend construidas en paralelo por varias personas y agentes, con desviaciones conocidas y otras sin detectar. Antes de la entrega oficial (1–4 dic) hace falta **evaluar todo con la misma vara** (la constitución), **corregir lo que no la cumple** y **demostrar con evidencias** que el backend está bien hecho. Esta spec no agrega funcionalidad: audita, consolida y cierra brechas de calidad.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P0 | Como equipo, queremos saber qué spec cumple la constitución y cuál no, con evidencia, para corregir antes de entregar. |
| H-02 | P0 | Como equipo, queremos corregir la deuda técnica conocida sin romper el contrato con la app. |
| H-03 | P0 | Como equipo académico, queremos demostrar las siete categorías de pruebas (unitarias, integración, contrato, base de datos, mutación, rendimiento y seguridad) con resultados reales. |
| H-04 | P1 | Como persona que revisa, quiero ver de cada requisito qué prueba lo cubre. |
| H-05 | P1 | Como equipo, queremos una demostración en staging con datos suficientes para que cada función se vea funcionando. |

## 3. Requisitos

### A. Evaluar (sin cambiar código)
| ID | Requisito |
|---|---|
| RF-01 | **Inventario de specs** (`docs/calidad/inventario.md`): por cada spec de backend (001–015 y 017) su estado declarado, sus tareas hechas y abiertas, si su tarea de despliegue pasó, su veredicto de conformidad y las tareas abiertas con su dueño. Línea base del 2026-11-17: 012 con 8 abiertas, 017 con 4, 013 con 2, 001 con 1; 009, 010 y 011 marcadas «Borrador» aunque están desplegadas. |
| RF-02 | **Matriz de conformidad** (`docs/calidad/conformidad.md`): spec × artículo de la constitución (I–XV y §5–§7) con ✅, ⚠️ (con observación) o ❌, y la evidencia de cada celda (archivo, prueba o comando). Se hace con el protocolo de `revisor-constitucion` y comprobaciones automáticas donde existan (prueba de arquitectura, `gofmt`, `golangci-lint`, cobertura, `gitleaks`, `govulncheck`). |
| RF-03 | **Trazabilidad** (`docs/calidad/trazabilidad.md`): cada `RF`, `RNF` y `CA` de las specs 003–015 con la prueba que lo cubre (paquete y nombre) o «sin prueba». Se genera en parte con un comando que busca las referencias `CA-NN`/`RF-NN` en los nombres y comentarios de las pruebas, y se completa a mano. |
| RF-04 | **Registro de deuda técnica** (`docs/calidad/deuda.md`): cada hallazgo con ID (`DT-NN`), spec, artículo, gravedad (P0 bloquea la entrega, P1 se corrige en esta spec, P2 se documenta), dueño y estado. Entra la deuda ya conocida: (a) `validator/v10` solo se usa en 5 de 15 módulos HTTP (Art. VI.2; faltan bookings, chat, handover, incidents, notifications, payments, ratings, reports, sanctions y search); (b) `RequireActive` existe pero las rutas de 009 no lo montan; (c) el buscador del asistente (014) filtra por la categoría que propone el modelo y, si se equivoca, no encuentra el objeto; (d) estados de spec desactualizados; (e) POL-06 y POL-07 provisionales; (f) staging sin datos de demostración; (g) foto de prueba huérfana en Cloudinary de staging. |

### B. Corregir
| ID | Requisito |
|---|---|
| RF-05 | Toda deuda **P0 y P1** se corrige en esta spec, **en el módulo de su dueño** (Art. XII.1: el dueño corrige o aprueba el cambio en el PR). Ninguna corrección cambia el contrato de forma incompatible (Art. IV.6); si un cambio de contrato es inevitable, se publica antes en `api/paths/` y se avisa a móvil. |
| RF-06 | **Validación en la frontera** (DT-a): los DTO de entrada de los 10 módulos faltantes usan `validator/v10` con las mismas reglas que ya aplica el dominio; los errores salen como `validation_failed` con `details` por campo (Art. IV.3). |
| RF-07 | Se corrigen los estados de spec, se confirman o ajustan POL-06 y POL-07 con el equipo, y se borra la foto huérfana (DT-d, e, g). |

### C. Cerrar brechas de pruebas (las siete categorías)
| ID | Requisito |
|---|---|
| RF-08 | **Pruebas de contrato:** hoy solo se valida que `openapi.yaml` esté bien escrito (redocly). Se agrega una verificación de que **cada respuesta real** de la prueba de punta a punta (012) y de las pruebas de integración de los handlers cumple su esquema de `openapi.yaml` (código, cuerpo y error del Art. IV). Una respuesta que no cumple hace fallar la CI. |
| RF-09 | **Mutación en todos los módulos:** `scripts/mutation.sh` hoy solo recorre `internal/domain`, `internal/usecase` e `internal/platform/security`. Pasa a recorrer todos los `internal/*/domain` e `internal/*/usecase` con los umbrales actuales (eficacia ≥ 75 %, cobertura de mutantes ≥ 70 %), sin cambiar sus disparadores (semanal y a mano). Los paquetes bajo el umbral entran al registro de deuda. |
| RF-10 | **Seguridad dinámica (DAST):** análisis OWASP ZAP *baseline* (pasivo, sin ataques) contra staging, a mano desde Actions; las alertas Medias o Altas entran al registro de deuda. Nunca contra producción. |
| RF-11 | **Imagen y dependencias:** escaneo de la imagen Docker de la API (vulnerabilidades del sistema y de Go) y un **SBOM** CycloneDX por versión, en la CI; vulnerabilidades críticas o altas con corrección disponible hacen fallar la CI. |
| RF-12 | **Informe de las siete categorías** (`docs/calidad/pruebas.md`): para cada categoría, qué se prueba, dónde, cómo se ejecuta y el último resultado real (cobertura, mutación, p95 de carga, alertas de ZAP, vulnerabilidades). Nunca se reporta un resultado que no se ejecutó. |

### D. Demostrar y cerrar
| ID | Requisito |
|---|---|
| RF-13 | **Staging de demostración:** `seed` (012) y `seed-interactions` (015) cargados en staging, de modo que la búsqueda, el asistente (014) y las recomendaciones (015) muestren resultados; guion de demostración en `docs/calidad/demo.md`. |
| RF-14 | **Informe final de calidad** (`docs/calidad/informe.md`) para la entrega académica: resumen de conformidad, deuda corregida y pendiente, siete categorías con resultados, y riesgos conocidos. Lo firman BE-A y BE-B. |
| RNF-01 | Sin funcionalidad nueva ni cambios incompatibles de contrato. |
| RNF-02 | Herramientas gratuitas y sin servicios de pago (Art. XIII.1). Una dependencia de Go nueva solo si se justifica en `plan.md` y se registra en `memoria.md` (Art. VIII.1). |
| RNF-03 | Los documentos de `docs/calidad/` no contienen secretos ni datos personales. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado el inventario y la matriz, entonces cada spec de backend tiene estado, tareas y veredicto con evidencia, y ninguna celda dice ✅ sin evidencia. |
| CA-02 | Dado el registro de deuda, entonces no queda ninguna deuda P0 ni P1 abierta, y las P2 tienen dueño y justificación. |
| CA-03 | Dada una petición con un campo inválido a cualquiera de los 10 módulos corregidos, entonces responde 400 `validation_failed` con `details` del campo. |
| CA-04 | Dada una respuesta que no cumple su esquema de `openapi.yaml`, entonces la CI falla y dice qué ruta y qué campo. |
| CA-05 | Dado el informe de mutación, entonces todos los `domain` y `usecase` de módulo aparecen, y los que no alcanzan el umbral están en el registro de deuda. |
| CA-06 | Dado el análisis ZAP de staging, entonces no quedan alertas Altas abiertas y las Medias están corregidas o justificadas. |
| CA-07 | Dada la imagen de la API, entonces hay SBOM y no hay vulnerabilidades críticas o altas con corrección disponible. |
| CA-08 | Dado staging, entonces el guion de demostración se puede seguir completo: búsqueda con resultados, asistente con coincidencias cercanas y recomendaciones con `reason`. |
| CA-09 | Dado el informe de las siete categorías, entonces cada resultado tiene fecha y enlace a la ejecución que lo produjo. |

## 5. Fuera de alcance
Funcionalidad nueva; cambios a la constitución (si la auditoría encuentra que hace falta uno, se propone por escrito, Art. XI); pruebas de penetración activas o contra producción; servicios de pago; el repositorio móvil y la landing (tienen su propia constitución; se pueden auditar con esta misma plantilla en su repositorio).

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: para las pruebas de contrato, ¿se acepta una dependencia **solo de pruebas** que valide respuestas contra OpenAPI 3.1 (propuesta en el plan, P-03), o se prefiere una validación propia más limitada con la biblioteca estándar?]
- [NEEDS CLARIFICATION: reparto final de las correcciones entre BE-A y BE-B; propuesta: cada dueño corrige sus módulos según `ROADMAP.md` §2.]
- Pendiente del equipo: confirmar o ajustar POL-06 (QR de 15 min) y POL-07 (tolerancia de 2 h).
