# Spec 003 — Cuenta y autenticación (backend)

- **Dueño:** BE-A · **Prioridad:** P0 · **Ventana:** 31 oct–2 nov · **Depende de:** 001, 002
- **Constitución:** v1.3.0 · **Políticas:** POL-12, POL-17 · **Estado:** Borrador

## 1. Objetivo y por qué
Permitir que una persona cree su cuenta, inicie sesión y administre su perfil, y dejar listas las piezas que usan todas las demás specs: identidad en el contexto, roles, módulos registrables y puertos vacíos (`notify.Notifier`, `moderation.Checker`). Una misma cuenta puede publicar y solicitar.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona nueva, quiero registrarme con correo y contraseña, para usar VAIVÉN. |
| H-02 | P1 | Como persona registrada, quiero iniciar y cerrar sesión de forma segura, para proteger mi cuenta. |
| H-03 | P1 | Como persona usuaria, quiero que mi sesión se renueve sola, para no iniciar sesión a cada rato. |
| H-04 | P1 | Como persona usuaria, quiero ver y editar mi perfil, para que otras personas me reconozcan. |
| H-05 | P2 | Como persona usuaria, quiero ver el perfil público de otra persona, para decidir si confío en ella. |
| H-06 | P2 | Como persona usuaria, quiero dar de baja mi cuenta, para dejar de usar el servicio. |
| H-07 | P1 | Como **equipo**, quiero un registro de módulos y puertos compartidos, para que 4 personas agreguen funcionalidad sin conflictos. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `POST /api/v1/auth/register` crea la cuenta con `email`, `password`, `displayName`, `acceptTerms=true`, `isAdult=true` (POL-12) y devuelve usuario + tokens (201). |
| RF-02 | El correo se normaliza (minúsculas, sin espacios) y es único; duplicado → 409 `conflict`. |
| RF-03 | Contraseña: 8–72 caracteres; se guarda con `argon2id`; nunca se devuelve ni se registra. |
| RF-04 | `POST /api/v1/auth/login` devuelve usuario + tokens; credenciales inválidas → 401 `unauthorized` con mensaje genérico (no revela si el correo existe). |
| RF-05 | Tokens: **acceso** JWT de 15 min y **refresco** opaco de 30 días, rotativo: cada `refresh` entrega uno nuevo e invalida el anterior; reutilizar uno ya usado revoca toda su familia. |
| RF-06 | `POST /api/v1/auth/refresh` y `POST /api/v1/auth/logout` (204; revoca la familia). |
| RF-07 | Límite de intentos: 10 peticiones por minuto por IP en `register`, `login` y `refresh` → 429 `rate_limited`. |
| RF-08 | `GET /api/v1/me`, `PATCH /api/v1/me` (`displayName` 2–40 caracteres, `phone` opcional) y `DELETE /api/v1/me` con `password` (POL-17): anonimiza correo, nombre y teléfono, revoca sesiones y deja de aceptar inicio de sesión. |
| RF-09 | `GET /api/v1/users/{id}` devuelve perfil público: `id`, `displayName`, `ratingAverage`, `ratingCount`, `memberSince`. Cuenta dada de baja → `displayName = "Cuenta eliminada"`. |
| RF-10 | Roles `user` y `admin`; middleware `Authenticate` y `RequireRole`; `go run ./cmd/admin create-admin` crea un administrador pidiendo la contraseña por entrada estándar. |
| RF-11 | Registro de módulos en `cmd/api/modules.go` (interfaz `Module{ Name; Register(chi.Router) }`). |
| RF-12 | Puertos con implementación vacía: `notify.Notifier` (no hace nada) y `moderation.Checker` (permite todo). |
| RF-13 | Un usuario con `suspendedUntil` en el futuro recibe 403 `forbidden` al **crear** objetos o solicitudes (el middleware `RequireActive`). |
| RNF-01 | Contraseñas, tokens y hashes nunca aparecen en logs ni respuestas de error. |
| RNF-02 | Cobertura ≥ 80 % en `auth` (usecase y domain). |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado un correo nuevo, cuando me registro, entonces recibo 201 con tokens y puedo llamar `/me`. |
| CA-02 | Dado un correo ya usado (con otras mayúsculas), cuando me registro, entonces recibo 409. |
| CA-03 | Dado un `refreshToken` válido, cuando lo uso dos veces, entonces la segunda falla con 401 y la familia queda revocada. |
| CA-04 | Dado un token de acceso vencido, cuando llamo `/me`, entonces recibo 401 `unauthorized`. |
| CA-05 | Dado que hay reservas activas (cuando exista 007), cuando pido la baja, entonces recibo 409; sin reservas, 204 y ya no puedo iniciar sesión. |
| CA-06 | Dado el 11.º intento por minuto desde una IP, entonces recibo 429. |
| CA-07 | Dado un usuario que no es admin, cuando llamo una ruta de admin, entonces recibo 403. |

## 5. Fuera de alcance
Verificación de correo, recuperación de contraseña, inicio social, foto de perfil, calificaciones reales (010; aquí `ratingAverage` y `ratingCount` valen `null`/0).

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Se exige teléfono? Provisional: opcional y nunca visible en el perfil público.]
- Términos y privacidad: viven en la **landing** (`vaiven-landing-page` L001); el registro de la app abre sus URL públicas (la app no incluye el texto). [NEEDS CLARIFICATION: revisión legal de los textos por una persona antes de publicar.]
