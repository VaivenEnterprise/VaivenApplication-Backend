# Aceptación de gzip en Expo Go

La regresión backend se ejecuta con las 69 operaciones OpenAPI y negociación gzip. La aceptación del cliente corresponde a MO-A y MO-B con B-7. Este procedimiento no cambia el contrato ni requiere modificar la app.

1. Anotar versión de la app, Expo Go, sistema y entorno HTTPS autorizado. Usar una cuenta sintética y un catálogo suficientemente grande para superar el umbral de compresión. Mantener constantes la cuenta y los datos al comparar.
2. En el cliente, recorrer búsqueda, paginación y ficha. Verificar que el transporte anuncia gzip, obtiene HTTP 200 y presenta los mismos datos en dos corridas, con compresión habilitada y deshabilitada. Comparar el JSON descomprimido, incluyendo cursores, fechas, identificadores y estados nulos.
3. Probar inicio de sesión, renovación de sesión, perfil privado, firma de subida y las pantallas que solicitan QR donde existan datos sintéticos autorizados. Confirmar que las rutas excluidas carecen de `Content-Encoding: gzip`. Registrar códigos y encabezados de negociación sin cuerpos privados, credenciales ni URLs firmadas.
4. Confirmar en las respuestas comprimidas `Content-Encoding: gzip` y `Vary: Accept-Encoding`. Verificar por separado que un cliente que pide `identity` recibe el mismo JSON sin compresión.
5. Registrar éxito o fallo por versión y caso. Cualquier cambio semántico o error de lectura bloquea habilitar gzip en staging para esa versión del cliente.
