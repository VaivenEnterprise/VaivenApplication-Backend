# Plan de validación con personas usuarias

> Spec 012, T010 (RF-10). Es un **plan**: describe qué observar y cómo medirlo durante la prueba cerrada de Google Play (14 días). Todavía **no hay resultados**; nada de lo de abajo es un dato comprobado.

## 1. Qué queremos saber

¿Las personas pueden publicar, encontrar, pedir y devolver objetos sin ayuda? ¿El préstamo o la renta se completan de punta a punta? ¿Dónde se atoran?

## 2. Qué observar y de dónde sale el dato

Todo se obtiene con conteos sobre MongoDB de producción o de la prueba cerrada, **sin datos personales**: solo cantidades, estados y tiempos.

| Señal | Cómo se mide | Por qué importa |
|---|---|---|
| Objetos publicados | `items` activos por semana y por categoría (POL-14) | Si hay oferta suficiente para que la búsqueda sea útil |
| Solicitudes y aceptación | `bookings` por estado: `pending`, `accepted`, `rejected`, `expired`, `cancelled` | Una tasa alta de `expired` indica que los dueños no responden a tiempo (POL-03, 48 h) |
| Recorridos completos | Reservas que llegan a `completed` frente a las aceptadas | Si el flujo de entrega y devolución con QR se entiende |
| Devoluciones | Reservas con `isLate` y días de retraso (POL-07) | Si el plazo y la tolerancia de 2 h son realistas |
| Tiempo de respuesta del dueño | Mediana entre la creación de la solicitud y su aceptación o rechazo | Si conviene recordar con avisos o ajustar POL-03 |
| Chat | Mensajes por reserva aceptada | Si las partes coordinan dentro de la app |
| Incidencias | `incidents` abiertas y cerradas, y su tiempo de cierre | Si la administración da abasto y si hay problemas recurrentes |
| Reportes de contenido | `reports` por tipo y por resolución (POL-16) | Si el filtro de palabras deja pasar o bloquea de más |
| Rendimiento real | p95 de `GET /items` y `GET /me/bookings` en los registros de la API | Comparar con la medición local de `docs/rendimiento.md` |

## 3. Cómo se hace

1. **Antes:** acordar qué cuenta como «uso real»: excluir las cuentas del `seed` y las del equipo.
2. **Durante (14 días):** tomar los conteos una vez por semana. No se instala analítica nueva ni se guardan datos personales.
3. **Sesiones observadas:** entre 3 y 5 personas ajenas al proyecto completan, con la app, tres tareas: publicar un objeto, solicitar uno y calificar. Se anota dónde dudan y cuánto tardan, sin grabar su pantalla ni sus datos.
4. **Al final:** un resumen breve con los números de la sección 2 y las tres fricciones más repetidas, separando lo **observado** de lo que solo se **supone**.

## 4. Qué no hace este plan

- No promete metas (por ejemplo, «el 80 % completará una reserva»): solo define qué medir.
- No recolecta datos personales ni analítica de terceros (Art. VI).
- No cambia la funcionalidad durante la prueba: los hallazgos se anotan para la v1.1.
