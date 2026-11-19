# Revocación de JWT · Propuesta para BE-A

La SPEC-018 sigue en propuesta. La prueba del laboratorio usa el verificador Go de la API y Redis real con llaves efímeras. El código de autenticación y el esquema de las sesiones se mantienen en la línea base.

## Contrato interno propuesto

1. El verificador valida firma y algoritmo, vencimiento y, tras la transición aprobada, `iss` por entorno y `aud` de la aplicación. Devuelve al consumidor el `jti`, el `iat` y un identificador de familia cuando esté disponible. El `AccessClaims` actual solo entrega usuario, rol y expiración, por lo que el fixture vuelve a verificar los mismos bytes para obtener los metadatos. La implementación productiva debe obtener ambos resultados en una sola validación.
2. MongoDB conserva la revocación durable por familia o cuenta. Logout y reutilización revocan la familia afectada. La baja invalida la cuenta. Los registros son idempotentes, preservan la versión más reciente y expiran como mínimo al superar la vida máxima de un JWT afectado.
3. Redis acelera la lectura de decisiones. Una entrada de autorización ausente, vencida o inaccesible consulta MongoDB. La vigencia total y el retardo de propagación deben sumar como máximo 60 segundos. Si las dos fuentes quedan inaccesibles, la ruta protegida devuelve un error temporal. Las rutas públicas permanecen disponibles.
4. BE-A define la transición para JWT previos sin familia, emisor o audiencia. Propuesta de revisión: aceptación acotada a la vida máxima del JWT antiguo, con una regla explícita de revocación para esos tokens y fecha de retiro comprobable.

El prototipo Redis actual conoce revocación de un `jti` y corte por usuario. La revocación de familia requiere una clave de familia o un registro MongoDB equivalente. Revocar un JWT no invalida todos los JWT de la misma familia. Un corte por usuario para logout sí invalidaría sesiones que deben seguir vigentes.

## Aceptación de la integración

| Caso | Evidencia que debe capturarse |
|---|---|
| Logout de una familia | Dos familias sintéticas del mismo usuario y dos JWT vigentes de la primera. Revocar la primera mediante el flujo actual. Ambas réplicas rechazan ambos JWT afectados en hasta 60 s y la segunda familia sigue vigente. |
| Baja de cuenta | Dos familias del mismo usuario. Tras la baja registrada en MongoDB, ambas réplicas rechazan todas las sesiones afectadas en hasta 60 s. Otro usuario sigue activo. |
| Reutilización | El mismo refresh sintético se usa en el flujo de prueba. La familia queda revocada una sola vez en MongoDB y sus JWT dejan de funcionar en ambas réplicas. La otra familia permanece activa. |
| Redis inaccesible o reiniciado | Repetir la lectura antes y después de indisponibilidad, reinicio y caché vacía. Mantener el resultado durable de MongoDB. Confirmar el error temporal de ruta protegida si ambas fuentes fallan. |
| Expirado y firma incorrecta | La validación de firma y expiración rechaza un JWT vencido y otro emitido con una llave efímera distinta antes de consultar la política de revocación. |
| Emisor, audiencia y transición | Control con tokens sintéticos del entorno correcto e incorrecto, audiencia incorrecta y tokens antiguos antes y después de la fecha de retiro. |
| Medición | Registrar instante de confirmación durable y primer rechazo en cada réplica, con una cota de 60 s. Los informes solo guardan estados, duraciones y correlación sintética. |

## Evidencia local

`internal/platform/redis/lab018_jwt_integration_test.go` prueba firma y expiración con el verificador de la API, dos clientes de Redis, revocación por `jti`, corte de cuenta e indisponibilidad. Las acciones del fixture escriben directamente al almacén para probar el límite entre validación y revocación. Las pruebas de logout, baja y reutilización a través de los endpoints quedan condicionadas a la integración que apruebe BE-A.
