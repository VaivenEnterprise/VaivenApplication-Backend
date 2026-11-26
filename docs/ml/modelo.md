# Modelo de recomendaciones «Recomendado para ti» (spec 015, RF-11)

Documento para la defensa académica: qué datos usa el modelo, cómo calcula, qué supuestos hace, qué límites tiene y por qué no se usa una red neuronal. La evaluación con números está en [evaluacion.md](evaluacion.md).

## 1. Qué hace

Sugiere objetos para rentar o pedir prestados a partir de lo que la persona ha visto y solicitado. **Nunca reemplaza la búsqueda**: solo sugiere, y cada sugerencia dice por qué aparece (`reason`). Se implementó en Go y MongoDB, sin servicios externos ni costo, y se apaga con `RECO_ENABLED`.

## 2. Datos

| Dato | Origen | Peso | Retención |
|---|---|---|---|
| Vista del detalle de un objeto | `POST /recommendations/events` (la app) | 1 | 90 días (TTL) |
| Solicitud de reserva | colección `bookings` de la spec 007, en solo lectura | 3 | ventana de 90 días |
| Categoría, modalidad, tarifa y municipio del objeto | colección `items` de la spec 005, en solo lectura | — | — |

- Una persona y un objeto cuentan **una sola vista cada 10 minutos**, para que abrir varias veces el mismo detalle no infle la señal.
- No se guardan la ubicación exacta, textos ni datos de contacto: solo identificadores, tipo, peso y fecha.
- La persona puede borrar su historial (`DELETE /recommendations/history`). Sus vistas se borran y sus solicitudes anteriores dejan de contar.
- Al recalcular el modelo se purgan los eventos de las cuentas dadas de baja.

## 3. Fórmulas

**Matriz de interacciones.** `w(u, i)` es el peso máximo de las interacciones de la persona `u` con el objeto `i` en los últimos 90 días.

**Colaborativo objeto-objeto (RF-02).** Dos objetos se parecen si las mismas personas interactúan con ambos:

```
cf(i, j) = Σ_u w(u,i)·w(u,j) / ( ‖i‖ · ‖j‖ ),    ‖i‖ = √( Σ_u w(u,i)² )
```

Es la similitud coseno entre las columnas de la matriz. Solo se suman personas que tocaron los dos objetos, así que el costo es Σ_u n_u² (n_u = objetos de la persona) y no objetos². Se guardan los 20 vecinos más parecidos de cada objeto.

**Contenido (RF-03).** Cada objeto es un vector *one-hot* con cuatro rasgos:
- categoría;
- modalidad (préstamo o renta);
- banda de tarifa diaria: préstamo, ≤ $50, ≤ $200, ≤ $500 o > $500;
- municipio.

Con vectores *one-hot* el coseno se reduce a:

```
contenido(i, j) = rasgos_compartidos / √( rasgos_i · rasgos_j )
```

Para que el costo sea acotado, solo se comparan objetos que comparten categoría o municipio.

**Híbrido (RF-04).** Para cada candidato `c`, sumando sobre los 20 objetos más recientes `h` del historial:

```
score(c) = Σ_h  w(h) · [ α·cf(h, c) + (1 − α)·contenido(h, c) ],    α = 0.6
```

El motivo `similar_to` es el objeto `h` que más contribuyó. α se ajustó una sola vez con la evaluación, de 0.7 a 0.6.

**Arranque en frío.** Con menos de 2 objetos en el historial se usan, en este orden y sin repetir:
1. Los populares del municipio que muestra la app (`popular_nearby`).
2. Los populares de las categorías que vio (`popular_in_category`).
3. Los populares generales (`popular`).

La popularidad es la suma de pesos de cada objeto; los objetos sin interacciones van al final, del más nuevo al más viejo, para que la lista nunca quede vacía si hay objetos visibles.

**Reglas finales.**
- **Exclusiones:** nunca se recomiendan objetos del historial, en reserva activa, propios, no activos o de cuentas suspendidas o dadas de baja. Las tarjetas salen de la búsqueda de la spec 006, con sus mismas reglas de visibilidad.
- **Diversidad:** máximo 3 por categoría.

## 4. Cálculo y operación

- El modelo (vecinos y populares) se **precalcula** y se guarda en MongoDB, en `reco_neighbors` y `reco_popular_lists`. Una petición solo lee y combina, por eso es rápida: el p95 local es de unos 6 ms.
- **Recálculo:** se hace con el comando `recompute-recommendations` o de forma perezosa cuando el modelo tiene más de 6 h.
  - Lo protege un **candado en MongoDB** que vence a los 2 min: solo un proceso recalcula, aunque haya varias réplicas.
  - **La petición nunca espera el recálculo.**
  - El modelo nuevo se escribe junto al anterior y se vuelve vigente en una sola escritura, así que nunca se lee un modelo a medias.
- Con 5 000 objetos y 50 000 interacciones, el recálculo tarda unos 0,5 s y asigna unos 32 MB (*benchmark*); el límite es 5 s y 100 MB.
- Todo el cálculo es **determinista**: suma flotantes siempre en el mismo orden y desempata por identificador, así que los mismos datos dan siempre el mismo modelo.

## 5. Supuestos

1. Si muchas personas interactúan con dos objetos, esos objetos se parecen para quien interactúa con uno de ellos.
2. Una solicitud expresa más intención que una vista (peso 3 contra 1).
3. Los objetos de la misma categoría, modalidad, rango de precio y municipio son sustitutos razonables.
4. Los gustos cambian poco en 90 días, y lo reciente pesa más (historial de 20).

## 6. Límites

- **Sesgo de popularidad.** Los objetos populares aparecen más y por eso reciben más interacciones. Lo mitigan el modelo de contenido y el límite de 3 por categoría.
- **Pocos datos.** Con pocas personas, el colaborativo casi no tiene señal. Por eso existen el modelo de contenido y el arranque en frío.
- **Objeto nuevo.** Un objeto sin interacciones solo entra por contenido o por ser nuevo en la lista de populares.
- **Datos sintéticos.** La evaluación usa datos sintéticos (ver [evaluacion.md](evaluacion.md)). No mide la experiencia de personas reales.
- **No hay aprendizaje en línea.** El modelo cambia al recalcularse, como máximo cada 6 h.

## 7. Por qué no una red neuronal

- **Datos.** Una red neuronal (por ejemplo, factorización neuronal o *embeddings*) necesita muchas más interacciones de las que tendrá VAIVÉN al inicio. Con pocos datos, el filtrado colaborativo objeto-objeto y el contenido funcionan mejor y no sobreajustan.
- **Explicación.** Cada recomendación se puede explicar («parecido a lo que viste»), lo que pide la spec (H-02) y ayuda a la confianza.
- **Costo e infraestructura.** Corre en la misma VM gratuita (~1 GiB de RAM), sin GPU, sin servicios externos y sin dependencias nuevas (Art. VIII y XIII.1).
- **Evaluación.** Las métricas y la línea base de popularidad permiten demostrar que el modelo aporta, sin depender de una caja negra.
