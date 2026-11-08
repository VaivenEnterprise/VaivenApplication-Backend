# Spec 009 — Entrega, devolución y pagos registrados (backend)

- **Dueño:** BE-A · **Prioridad:** P0 · **Ventana:** 13–17 nov · **Depende de:** 004, 007
- **Constitución:** v2.0.0 · **Políticas:** POL-05, 06, 07, 08, 13 · **Estado:** Borrador

## 1. Objetivo y por qué
Documentar el momento más delicado: la entrega y la devolución **presenciales**, con estado del objeto, fotografías y confirmación de ambas partes mediante QR, y registrar el pago acordado. El QR confirma una etapa; **no** confirma el pago, la identidad ni el estado por sí solo.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como propietario, quiero registrar el estado y las fotos del objeto al entregarlo, para tener evidencia. |
| H-02 | P1 | Como solicitante, quiero revisar esa evidencia y confirmar la recepción con un QR, para dejar constancia. |
| H-03 | P1 | Como solicitante, quiero registrar el estado al devolver, y como propietario, confirmar la recepción, para cerrar la operación. |
| H-04 | P1 | Como persona usuaria, quiero reportar y confirmar el pago acordado, para llevar el control del dinero fuera de la app. |
| H-05 | P2 | Como persona usuaria, quiero que una devolución tardía quede registrada con su ajuste y se avise a quien espera el objeto. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `PUT /api/v1/bookings/{id}/delivery/evidence` (propietario; reserva `accepted`): `conditionNotes` (3–500) y `photos` 1–5 (`{publicId}`, propósito `evidence`). Reemplazar la evidencia incrementa `evidenceVersion` e **invalida** el QR vigente. |
| RF-02 | `POST /api/v1/bookings/{id}/delivery/qr` (propietario; con evidencia): genera un QR de **un solo uso**, vigente 15 min (POL-06), ligado a reserva + etapa + versión de evidencia; generar otro invalida el anterior. Devuelve `token`, `qrPayload` y `expiresAt`. |
| RF-03 | `POST /api/v1/bookings/{id}/delivery/confirm` (solicitante) con `token`: valida vigencia, uso único, etapa, versión y que quien llama es el solicitante; la reserva pasa a `in_progress`. |
| RF-04 | La devolución es simétrica: el **solicitante** registra evidencia (`PUT /return/evidence`) y genera el QR (`POST /return/qr`); el **propietario** confirma (`POST /return/confirm`); la reserva pasa a `completed`. |
| RF-05 | `GET /api/v1/bookings/{id}/delivery` y `/return` devuelven la etapa a las dos partes, con fotos mediante **URLs firmadas de 10 min**. Terceros → 404. |
| RF-06 | Un QR vencido, usado o de otra etapa/reserva se rechaza (409 `conflict` o 400 con motivo claro); se puede **renovar** generando otro. La evidencia previa se conserva. |
| RF-07 | Pagos: `POST /bookings/{id}/payments/{stage}/report` (solicitante, `note` ≤ 200) y `POST /bookings/{id}/payments/{stage}/confirm` (propietario) con `stage` ∈ {`delivery`,`return`}. Estados `pending → reported → confirmed`. Importes los fijados al solicitar (007). El QR **no** modifica el pago y el pago **no** condiciona el QR. |
| RF-08 | Préstamos gratuitos no tienen pagos. |
| RF-09 | **Retraso:** pasada la fecha de fin + tolerancia de 2 h (POL-07) sin devolución, la reserva se marca `isLate`; se notifica a ambas partes y, si existe otra reserva `accepted` del mismo objeto que empieza en el periodo afectado, se marca como afectada y se notifica a esa persona. |
| RF-10 | Al confirmar la devolución tardía se registra un **ajuste** `{type:"late_return", lateDays, amountCents, status:"recorded"}` (días de retraso redondeados hacia arriba × tarifa diaria; solo renta). **No se cobra**; es distinto de un pago recibido. |
| RF-11 | Una persona con reserva afectada puede cancelar sin penalización (`cancel` de 007 con motivo `retraso_devolucion`); no se elimina descuento ni compensación al siguiente usuario (POL del PDF §02). |
| RF-12 | Cada etapa confirmada notifica a la otra parte (`notify.Notifier`). |
| RF-13 | Cambiar la evidencia después de confirmada una etapa **no** es posible; las discrepancias se tratan con incidencias (010). |
| RNF-01 | El token QR se guarda solo como hash (SHA-256); no aparece en logs. |
| RNF-02 | Confirmar con el mismo QR dos veces a la vez: exactamente una tiene éxito. |
| RNF-03 | Cobertura ≥ 80 % en `domain` y `usecase` de `handover` y `payments`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una reserva `accepted` con evidencia, cuando el propietario genera QR y el solicitante lo confirma, entonces la reserva queda `in_progress`. |
| CA-02 | Dado un QR de hace 16 min, cuando se confirma, entonces falla y puedo generar otro. |
| CA-03 | Dado un QR ya usado, cuando se reintenta, entonces falla. |
| CA-04 | Dado que cambié la evidencia, cuando se escanea el QR anterior, entonces falla. |
| CA-05 | Dada una reserva `in_progress`, cuando el solicitante registra evidencia y el propietario confirma el QR, entonces queda `completed`. |
| CA-06 | Dado un pago reportado, cuando el propietario lo confirma, entonces el estado es `confirmed`; sin reporte previo → 409. |
| CA-07 | Dada una devolución 1 día 3 h tarde en renta de $30/día, entonces el ajuste es 2 días × 3000 = 6000 registrado, sin cobro. |
| CA-08 | Dada una devolución tardía con otra reserva aceptada que empieza ese día, entonces esa reserva queda marcada como afectada y su solicitante recibe aviso. |
| CA-09 | Dada una persona ajena, cuando pide la evidencia, entonces 404. |

## 5. Fuera de alcance
Verificación de identidad, cobro o verificación automática de pago, depósitos, seguros, edición de evidencia confirmada, resolución de disputas (010).

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Se permite confirmar la entrega antes de `startDate`? Provisional: sí, desde 1 día antes del inicio; fuera de esa ventana, 409.]
- [NEEDS CLARIFICATION: confirmar POL-06 y POL-07.]
