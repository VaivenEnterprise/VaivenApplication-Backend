# Spec 007 — Solicitudes, precio y reservas (backend)

- **Dueño:** BE-A · **Prioridad:** P0 · **Ventana:** 10–13 nov · **Depende de:** 005
- **Constitución:** v2.0.0 · **Políticas:** POL-01, 02, 03, 05, 09, 17 · **Estado:** Implementada (a pedido del responsable el 2026-11-08, pendiente de visto bueno de BE-A; desviación del Art. I.1 registrada en memoria)

## 1. Objetivo y por qué
Convertir el interés en un acuerdo claro: la persona ve el importe y las condiciones **antes** de solicitar; el propietario acepta o rechaza; al aceptar se **reserva** el periodo sin cruces. La solicitud conserva lo que se mostró aunque el objeto cambie después.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como solicitante, quiero ver el importe total y las condiciones por fechas antes de enviar, para decidir sin sorpresas. |
| H-02 | P1 | Como solicitante, quiero enviar una solicitud para un periodo y una modalidad de pago, para pedir el objeto. |
| H-03 | P1 | Como propietario, quiero aceptar o rechazar solicitudes, para decidir a quién presto. |
| H-04 | P1 | Como propietario, quiero que al aceptar se reserve el periodo, para evitar acuerdos incompatibles. |
| H-05 | P1 | Como persona usuaria, quiero cancelar antes de la entrega con un motivo, para liberar la reserva. |
| H-06 | P2 | Como persona usuaria, quiero ver mis solicitudes y reservas por rol y estado, para dar seguimiento. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `GET /api/v1/items/{id}/quote?startDate&endDate&paymentMode` devuelve días, tarifa, total en MXN, desglose de pagos (POL-05) y textos de política (vencimiento, cancelación tardía y ausencia, POL-09). No crea nada. |
| RF-02 | `POST /api/v1/bookings` crea una solicitud `pending` con `itemId`, `startDate`, `endDate`, `paymentMode` y `acceptedTotalCents`; si el total actual difiere → 409 `conflict` (`quote_changed`). |
| RF-03 | Reglas: no se solicita un objeto propio; el objeto debe estar `active`; `startDate ≥ hoy`; `endDate ≥ startDate`; 1–7 días (POL-02, una semana); sin cruce con reservas `accepted`/`in_progress`; no se repite una solicitud pendiente propia que se cruce. |
| RF-04 | La solicitud guarda una **instantánea**: tarifa, total, modalidad, título, reglas de uso y primera foto. Un cambio posterior del objeto no la altera. |
| RF-05 | Préstamo: total $0, `paymentMode = none`, sin pagos. Renta: `paymentMode` ∈ {`full_on_delivery`, `split_50_50`} y los importes de POL-05 quedan registrados en la solicitud. |
| RF-06 | La solicitud vence a las 48 h o al llegar la fecha de inicio, lo primero (POL-03), sin sanción; pasa a `expired`. Si la fecha de inicio es hoy, vence al terminar ese día. [NEEDS CLARIFICATION: confirmar con BE-A; ajuste A-02 del plan] |
| RF-07 | `POST /api/v1/bookings/{id}/accept` (propietario) reserva el periodo de forma **atómica**: verifica ausencia de cruces dentro de una transacción; al aceptar, las demás solicitudes pendientes que se cruzan pasan a `rejected` con motivo `no_disponible`. Acepta `meetingPoint.description` (3–200) opcional. |
| RF-08 | `POST /bookings/{id}/reject` (propietario, `reason` opcional ≤ 200) y `POST /bookings/{id}/cancel` (cualquiera de las partes, solo `pending` por el solicitante o `accepted` por cualquiera; `reason` 3–200 obligatorio en `accepted`). Cancelar libera la reserva y conserva motivo e historial. |
| RF-09 | `POST /bookings/{id}/meeting-point` (propietario, reserva `accepted`/`in_progress`) fija o cambia el punto de encuentro; solo lo ven las dos partes. |
| RF-10 | `GET /api/v1/bookings/{id}` (partes o admin) y `GET /api/v1/me/bookings?role=owner|requester&status=&cursor=&limit=`. |
| RF-11 | Estados: `pending`, `accepted`, `rejected`, `expired`, `cancelled`, `in_progress`, `completed`; las transiciones válidas son las del plan §4 y cualquier otra → 409. |
| RF-12 | Al crear una solicitud se abre su conversación (`chat.Opener`) y se notifica al propietario (`notify.Notifier`); aceptar, rechazar, cancelar y vencer también notifican. |
| RF-13 | Cambiar fechas o condiciones de una solicitud existente **no** es posible; se cancela y se crea otra (el acuerdo requiere aceptación expresa). |
| RF-14 | Una persona con reservas `accepted`/`in_progress` no puede darse de baja ni eliminar el objeto (reemplaza los puertos vacíos de 003 y 005). |
| RNF-01 | Dos aceptaciones simultáneas de periodos que se cruzan: exactamente una gana. |
| RNF-02 | Cobertura ≥ 80 % en `domain` y `usecase` de `bookings`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado un objeto a $30/día, cuando cotizo del 10 al 12, entonces `days=3`, `total=9000`. |
| CA-02 | Dado `split_50_50` con total 9001, entonces entrega 4501 y devolución 4500. |
| CA-03 | Dado que cambió el precio tras cotizar, cuando envío con el total anterior, entonces 409 `quote_changed`. |
| CA-04 | Dado mi propio objeto, cuando solicito, entonces 403/400 con detalle. |
| CA-05 | Dadas dos solicitudes pendientes que se cruzan, cuando se aceptan al mismo tiempo, entonces una queda `accepted` y la otra falla con 409. |
| CA-06 | Dada una pendiente sin respuesta 48 h, entonces su estado es `expired`. |
| CA-07 | Dado que el propietario cambió el precio después, entonces mi solicitud conserva la tarifa original. |
| CA-08 | Dada una reserva aceptada, cuando se cancela con motivo, entonces el periodo vuelve a estar disponible en la búsqueda. |

## 5. Fuera de alcance
Entrega y devolución (009), pagos reportados (009), chat (008), penalizaciones monetarias automáticas, cambio de fechas negociado, descuentos.

## 6. Preguntas abiertas
- POL-02/03/05/09 confirmadas el 28 de oct (POL-02: máximo 7 días).
- [NEEDS CLARIFICATION: ¿Una persona puede tener más de una solicitud pendiente sobre objetos distintos para las mismas fechas? Provisional: sí.]
