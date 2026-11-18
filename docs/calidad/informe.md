# Informe final de calidad del backend (spec 016, RF-14)

- **Fecha:** 2026-11-17 · **Versión:** borrador para revisión
- **Firmas:**
  - **BE-B:** pendiente
  - **BE-A:** pendiente
- **Documentos de apoyo** (todos en `docs/calidad/`):
  - `inventario.md` (T001)
  - `conformidad.md` (T002, actualizada)
  - `trazabilidad.md` (T003)
  - `deuda.md` (T004, con su actualización)
  - `pruebas.md` (siete categorías)
  - `demo.md` (staging de demostración)

## 1. Resumen

El backend tiene 16 specs; 14 están desplegadas en staging.

- **Conformidad:** con la constitución v2.0.0, el veredicto es **CONFORME CON OBSERVACIONES** en las specs desplegadas. Hay dos excepciones:
  - la **012** (pruebas de punta a punta, cabeceras de seguridad y despliegue de cierre) sigue abierta con su dueño, BE-A;
  - la **017** no tiene formato de spec.
- **Lo que corrigió la 016:**
  - validación con `validator/v10` en la frontera de los 9 módulos que reciben cuerpo;
  - pruebas de contrato con respuestas reales (69 de 69 operaciones);
  - estados de spec desactualizados;
  - un falso positivo de la CI en Windows.
- **Lo que preparó sin ejecutar:** la mutación de todos los módulos, el análisis ZAP de staging y el escaneo de la imagen con su SBOM. Cada uno espera su corrida.

## 2. Conformidad

| Resultado | Specs |
|---|---|
| ✅ en todas las columnas | 004 |
| Solo observaciones aceptadas (Art. X o I.1) | 001, 002, 006, 008, 010, 011, 015 |
| Tareas abiertas con dependencia | 013 (T008 espera la reversión por otra persona; T009 depende de 012-T011) |
| Esperan el visto bueno de BE-A | 003, 005, 007, 014 (y los módulos de BE-A tocados en T005: bookings, handover, payments) |
| Observación de diseño documentada | 009 (sin `RequireActive`, DT-b) |
| No cumple | 012 (abierta), 017 (sin formato) |

## 3. Deuda técnica

**Corregida en la 016:**

| ID | Qué se corrigió |
|---|---|
| DT-a | Validación en la frontera |
| DT-d | Estados de spec, en parte |
| DT-j | Encabezado de la 002 |
| DT-l | Falso positivo por CRLF |
| DT-n | Pruebas de contrato |
| DT-o, DT-p | Mutación, ZAP, Trivy y SBOM: corregidas en código; falta su corrida |

**Pendiente, con dueño:**

| ID | Dueño | Qué falta |
|---|---|---|
| DT-k | BE-A y BE-B | La 012 |
| DT-d2 | BE-A | Visto bueno de BE-A |
| DT-e | Equipo | Confirmar POL-06 y POL-07 |
| DT-f | `devops-mcp` | `seed` en staging, bloqueado por el archivo de credenciales |
| DT-g | Responsable | Borrar la foto huérfana en Cloudinary |
| 013-T008 | Otra persona del equipo | Ejecutar la reversión en staging |

**Aceptada o diferida (P2):**

| ID | Tema |
|---|---|
| DT-b | `RequireActive` en la 009 |
| DT-c | Categoría en el buscador del asistente |
| DT-h | Art. X |
| DT-i | Art. I.1 |
| DT-j | Formato de la 017 |
| DT-m | IDs sin prueba que los cite |

## 4. Siete categorías de pruebas

| Categoría | Resultado | Fecha |
|---|---|---|
| Unitarias | 779 en verde; `domain` y `usecase` con ≥ 91,2 % de cobertura | 2026-11-17 |
| Integración | 914 en verde (con unitarias), MongoDB 7 real | 2026-11-17 |
| Contrato | 69/69 operaciones, 0 incumplimientos | 2026-11-17 |
| Base de datos | Repositorios, índices, transacciones y concurrencia, en verde | 2026-11-17 |
| Mutación | **Pendiente** de la corrida en Actions (32 paquetes nuevos en el alcance) | — |
| Rendimiento | p95 de 13,2 ms (búsqueda) y 6,7 ms (reservas) en local; 205 ms en recomendaciones en staging | 2026-11-11 y 2026-11-17 |
| Seguridad | `govulncheck` 0, Gitleaks 0, lint 0; CodeQL en la CI; ZAP, Trivy y SBOM **pendientes** de su primera corrida | 2026-11-17 |

Los enlaces de la CI se agregan cuando corra la PR de la 016 (CA-09).

## 5. Riesgos conocidos

| Riesgo | Impacto | Mitigación |
|---|---|---|
| La 012 sin cerrar | No hay prueba de punta a punta ni despliegue de cierre; faltan cabeceras de seguridad | Tareas de BE-A en la 012; la 018 propone criterios complementarios |
| Mutación sin medir en 32 paquetes | No se sabe si las pruebas de esos módulos detectan cambios pequeños | Corrida en Actions; los paquetes bajo el umbral pasan a deuda |
| Primera corrida de Trivy | Puede encontrar críticas o altas en la imagen base y bloquear la CI de la PR | Actualizar la imagen base o la dependencia; si no hay corrección, no bloquea (`ignore-unfixed`) |
| Staging con pocos datos | La demostración se ve pobre | Opción A de `demo.md` §2 |
| Módulos de BE-A tocados por BE-B (T005) | Art. XII.1 | Visto bueno de BE-A en la PR |

## 6. Firmas

| Persona | Fecha | Veredicto |
|---|---|---|
| BE-B | — | — |
| BE-A | — | — |
