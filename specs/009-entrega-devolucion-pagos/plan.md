# Plan 009 — Entrega, devolución y pagos (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A

## 1. Estructura
```
internal/handover/
  domain/{stage.go, qr.go, errors.go}
  usecase/{record_evidence.go, issue_qr.go, confirm.go, get_stage.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{stage_repo.go}
internal/payments/
  domain/{payment.go, lateness.go}
  usecase/{report.go, confirm.go, late_adjust.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{payment_repo.go}
internal/payments/jobs/late_detector.go
api/paths/handover.yaml · api/paths/payments.yaml
```
Todo se guarda **dentro** de `bookings` (documentos de 007) mediante repositorios propios de estos módulos que tocan solo sus campos (`delivery`, `return`, `payments`, `isLate`, `adjustments`, `affectedBy`).

## 2. Contrato
| Método | Ruta | Cuerpo → Respuesta |
|---|---|---|
| PUT | `/api/v1/bookings/{id}/delivery/evidence` | `{conditionNotes,photos:[{publicId}]}` → `Stage` |
| POST | `/api/v1/bookings/{id}/delivery/qr` | → `{token,qrPayload,expiresAt}` |
| POST | `/api/v1/bookings/{id}/delivery/confirm` | `{token}` → `Booking` |
| GET | `/api/v1/bookings/{id}/delivery` | → `Stage` |
| PUT/POST/POST/GET | `/api/v1/bookings/{id}/return/…` | igual, con roles invertidos (§4) |
| POST | `/api/v1/bookings/{id}/payments/{stage}/report` | `{note?}` → `Booking` |
| POST | `/api/v1/bookings/{id}/payments/{stage}/confirm` | → `Booking` |

```
Stage = { recordedAt|null, recordedById|null, conditionNotes, photos:[Image+url], evidenceVersion,
          qr:{ state:"none"|"active"|"expired"|"used", expiresAt|null },
          confirmedAt|null, confirmedById|null }
qrPayload = "vaiven://qr?b=<bookingId>&s=<delivery|return>&t=<token>"
Adjustment = { type:"late_return", lateDays, amountCents, status:"recorded" }
```
`Booking.delivery`, `Booking.return`, `Booking.isLate`, `Booking.adjustments`, `Booking.affectedBy` se llenan aquí (007 deja los campos en `null`/vacío).

## 3. Datos (campos dentro de `bookings`)
```
delivery / return: { recordedAt, recordedById, conditionNotes, photos[≤5 Image], evidenceVersion,
                     qr:{ tokenHash, expiresAt, usedAt, evidenceVersion }, confirmedAt, confirmedById }
payments[]: { stage, amountCents, status, reportedAt, reportedNote, confirmedAt }   # ≤ 2
isLate, lateAt, adjustments[]: ≤ 1, affectedBy (bookingId|null)
```
Índice extra: `{status:1,lateAt:1}` para el detector de retrasos.

## 4. Roles por etapa
| Etapa | Registra evidencia | Genera QR | Confirma (escanea) | Estado resultante |
|---|---|---|---|---|
| Entrega | propietario | propietario | solicitante | `in_progress` |
| Devolución | solicitante | solicitante | propietario | `completed` |

## 5. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Token = 32 bytes aleatorios base64url; se guarda `SHA-256`; comparación en tiempo constante | VI |
| P-02 | Confirmación atómica con `findOneAndUpdate` filtrando `status`, `qr.tokenHash`, `qr.usedAt:null`, `qr.expiresAt>now`, `qr.evidenceVersion==evidenceVersion`; actualiza `usedAt`, `confirmedAt`, `status` en una operación | II |
| P-03 | Evidencia: `images.Service.Resolve(ctx,userID,"evidence",refs)`; lectura: `URLs` firmadas de 10 min solo para las dos partes | VI.7 |
| P-04 | Ventana de entrega: desde `startDate−1 día` (México) hasta `endDate` | V |
| P-05 | Retraso: `lateAt = fin del día endDate (23:59:59 México) + 2 h`; `late_detector` corre cada 5 min con reloj inyectado | IX |
| P-06 | `lateDays = ceil((confirmedAt − fin del día endDate) / 24 h)`, mínimo 1 si pasó la tolerancia; ajuste = `lateDays × pricePerDayCents` (renta) | V |
| P-07 | Reserva afectada: otra reserva `accepted` del mismo objeto con `startDate ≤ hoy` y `startDate > endDate` de la tardía → `affectedBy = <id tardía>`; notificación `booking_affected` | XII |
| P-08 | Pagos: máquina `pending→reported→confirmed`; `report` solo solicitante, `confirm` solo propietario y solo si `reported` | III |
| P-09 | Los repositorios solo hacen `$set` de sus campos con filtro por `status`; nunca reemplazan el documento completo | II |

## 6. Pruebas
Unitarias: máquina de QR (vigencia, uso, versión, etapa), lateness (tabla: tolerancia, 1 día, 2 días, préstamo), roles, pagos. Integración (Mongo real): flujo completo entrega→devolución, **concurrencia** de 2 confirmaciones con el mismo token, evidencia reemplazada invalida el QR, URLs firmadas solo para partes, detector de retrasos con reloj falso (reserva afectada).

## 7. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Operaciones atómicas de un documento; sin transacción innecesaria |
| III | Reglas en `domain`/`usecase` |
| V | Ajuste en centavos, fechas de México |
| VI | Token hasheado, evidencia privada con URL de 10 min |
| XII | Usa `images.Service`, `Notifier`; no edita rutas ajenas |
