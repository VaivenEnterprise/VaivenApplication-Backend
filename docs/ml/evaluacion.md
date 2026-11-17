# Evaluación del recomendador (spec 015, RF-07)

> **Los datos de esta evaluación son sintéticos.** Se generan con una semilla fija para que el resultado sea reproducible y no contienen datos de personas reales. Cuando staging tenga interacciones suficientes, la misma evaluación se puede repetir con `--source db`.

## 1. Cómo se reproduce

```bash
go run ./cmd/admin eval-recommendations            # datos sintéticos, sin base ni configuración
go run ./cmd/admin eval-recommendations --source db # interacciones del entorno (nunca producción)
```

Las dos ejecuciones con datos sintéticos dan exactamente la misma salida (lo comprueba una prueba automática).

## 2. Método

- **Datos.** 300 personas sintéticas (`demo-NNN`), 600 objetos, 8 categorías, 6 municipios, 12 interacciones por persona (3 600 en total). Cada persona prefiere una o dos categorías y un municipio, y dentro de cada categoría unos objetos son mucho más populares que otros. El 15 % de las interacciones son solicitudes (peso 3) y el resto vistas (peso 1).
- **Partición *leave-last-out*.** De cada persona con al menos dos objetos distintos se oculta su última interacción; el modelo se entrena con el resto y tiene que «adivinar» el objeto oculto.
- **Métricas, con K = 10:**
  - **Hit@10:** fracción de personas con el objeto oculto entre los 10 primeros.
  - **Recall@10:** objetos ocultos recuperados entre objetos ocultos. Con *leave-last-out* hay uno solo por persona, así que coincide con Hit@10; se reporta porque la spec lo pide.
  - **MRR@10:** promedio de 1/posición del objeto oculto (0 si no está en el top 10). Premia acertar más arriba.
- **Modelos comparados.** Los cuatro completan su lista con los populares y ninguno recomienda objetos del historial, así que la comparación mide lo que cada uno agrega sobre la popularidad.
  - **Híbrido:** α·colaborativo + (1−α)·contenido.
  - **Solo colaborativo.**
  - **Solo contenido.**
  - **Popularidad:** la línea base.

## 3. Resultado (2026-11-17, α = 0.6)

| Modelo | Hit@10 | Recall@10 | MRR@10 |
|---|---|---|---|
| **Híbrido (α = 0.6)** | **0.263** | **0.263** | **0.098** |
| Colaborativo | 0.140 | 0.140 | 0.056 |
| Contenido | 0.203 | 0.203 | 0.084 |
| Popularidad (línea base) | 0.087 | 0.087 | 0.039 |

**CA-04 se cumple:** el híbrido acierta en 26,3 % de las personas, **3 veces** más que la popularidad (8,7 %), y pone el objeto correcto más arriba (MRR 0,098 frente a 0,039). También supera a cada modelo por separado: el contenido aporta cuando la persona tiene pocas interacciones y el colaborativo cuando otras personas comparten sus gustos.

## 4. Ajuste de α y de los pesos (una sola vez)

La spec dejó α = 0.7 y los pesos 3/1 como provisionales, para ajustarlos **una sola vez** con esta evaluación. Barrido de α con el pipeline real (20 vecinos por objeto):

| α | 0.3 | 0.4 | 0.5 | **0.6** | 0.7 | 0.8 | 0.9 |
|---|---|---|---|---|---|---|---|
| Hit@10 | 0.213 | 0.227 | 0.250 | **0.263** | 0.257 | 0.220 | 0.170 |
| MRR@10 | 0.087 | 0.094 | 0.095 | **0.098** | 0.096 | 0.080 | 0.065 |

**Decisiones:**
- **α pasa de 0.7 a 0.6.** Es el mejor valor en las dos métricas. La diferencia con 0.7 es **pequeña**: unas 2 personas de 300, dentro del ruido de una sola semilla. Se adopta porque no empeora nada, no porque sea una mejora fuerte. Los extremos sí son claramente peores, lo que confirma que mezclar los dos modelos vale la pena.
- **Pesos 3 (solicitud) y 1 (vista): se mantienen.** En un barrido exploratorio (con listas de vecinos más largas que las de producción), los pesos 1 y 2 dieron un poco más que 3. Pero en los datos sintéticos las solicitudes se generan igual que las vistas, así que **este conjunto no puede medir** si pedir un objeto es una señal más fuerte que verlo. Bajar el peso sería ajustar el modelo a una limitación de los datos de prueba. El peso 3 refleja que una solicitud es una intención más fuerte, y se revisará con datos reales.

## 5. Límites de esta evaluación

- Los datos son sintéticos y sus patrones los definió el equipo. Un buen resultado aquí indica que la implementación es correcta y que el modelo aprovecha los patrones, no que vaya a rendir igual con personas reales.
- Es una sola semilla. Las diferencias de 1 o 2 puntos no son significativas.
- *Leave-last-out* oculta un solo objeto por persona. No mide diversidad, novedad ni satisfacción.
- No hay prueba A/B (fuera de alcance de la spec).
