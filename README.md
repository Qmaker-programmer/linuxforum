# Linux Forum

Un foro minimalista y autónomo escrito en Go, con SQLite, sesiones por cookie, configuración vía JSON, y un sistema de comentarios anidados con podado inteligente.

## Características

- **Publicaciones** — Creación, visualización, eliminación (solo autor, con confirmación del título) y filtrado por fecha/título.
- **Filtrado** — Ordenar posts por fecha (asc/desc) o título (A-Z / Z-A) desde la página principal y desde los resultados de búsqueda.
- **Comentarios anidados** — Respuestas en árbol con profundidad arbitraria.
- **Podado inteligente** — Al eliminar un comentario, si todo su subárbol está muerto (solo `[eliminado]`), se elimina por completo, incluyendo ancestros muertos.
- **Autenticación** — Registro e inicio de sesión con contraseñas hasheadas (bcrypt, coste por defecto).
- **Sesiones** — Cookie configurable con soporte de expiración y limpieza automática de sesiones vencidas.
- **Guardado de posts** — Marca posts como favoritos (solo visibles para el usuario).
- **Búsquedas** — Búsqueda de publicaciones por título, búsqueda de usuarios por nombre (coincidencia parcial), búsqueda en comentarios.
- **Perfiles** — Perfil de usuario con descripción editable y cambio de nombre de usuario.
- **Rate limiting** — Configurable por JSON: máximo de requests por ventana de tiempo.
- **HTTPS** — Soporte nativo configurable vía JSON.
- **Todo en backend** — Sin JavaScript, solo formularios HTML y redirecciones del servidor.
- **SQLite** — Base de datos persistente con AUTOINCREMENT y WAL mode.

## Stack

- **Lenguaje:** Go 1.25+
- **Dependencias:** `golang.org/x/crypto` (bcrypt) y `github.com/mattn/go-sqlite3`.
- **Frontend:** HTML templates (`html/template`) sin JavaScript ni frameworks.
- **Base de datos:** SQLite con WAL mode.

## Instalación y uso

```bash
go run main.go
```

El servidor corre en `http://localhost:8080` (puerto configurable).

### Cuenta de administración por defecto

| Usuario | Contraseña |
|---------|-----------|
| admin   | 1234      |

## Configuración (`config.json`)

| Campo | Descripción | Default |
|---|---|---|
| `rate_limit` | Máximo de solicitudes por ventana de tiempo | `100` |
| `reset_minutes` | Minutos para reiniciar el contador de rate limit | `1` |
| `port` | Puerto del servidor | `8080` |
| `db_path` | Ruta al archivo de base de datos SQLite | `forum.db` |
| `https` | Habilitar HTTPS | `false` |
| `cert_file` | Ruta al certificado SSL | `cert.pem` |
| `key_file` | Ruta a la llave SSL | `key.pem` |
| `session_token_name` | Nombre de la cookie de sesión | `session_token` |
| `session_expire_minutes` | Minutos hasta expirar la sesión (0 = nunca) | `0` |

## Estructura del proyecto

```
linuxforum/
├── main.go              # Servidor completo (single file, ~1100 líneas)
├── config.json          # Configuración general del servidor
├── .gitignore
├── go.mod               # Módulo Go
├── go.sum               # Checksum de dependencias
├── README.md
├── LICENSE
└── web/
    ├── head.html        # Template <head> compartido
    ├── upbar.html       # Barra superior de navegación
    ├── index.html       # Página principal (lista de posts + filtros)
    ├── filtered.html    # Resultados de filtrado
    ├── post.html        # Vista de post + comentarios
    ├── search.html      # Resultados de búsqueda + filtros
    ├── user.html        # Perfil de usuario
    ├── login.html       # Inicio de sesión / registro
    ├── public.html      # Formulario de nuevo post
    ├── confirm.html     # Confirmación de eliminación de post
    └── tux.png          # Mascota Tux
```

## Rutas de la API

| Método | Ruta              | Descripción                               | Autenticación |
|--------|-------------------|-------------------------------------------|---------------|
| GET    | `/`               | Lista todas las publicaciones             | No            |
| GET    | `/filtered`       | Lista publicaciones ordenadas             | No            |
| GET    | `/view?id=N`      | Ver un post y sus comentarios             | No            |
| GET    | `/search?query=X` | Buscar posts por título                   | No            |
| GET    | `/search?user=X`  | Buscar usuarios por nombre                | No            |
| GET    | `/user?u=X`       | Ver perfil de usuario                     | No            |
| GET    | `/confirm?id=N`   | Página de confirmación para eliminar post | No*           |
| POST   | `/post`           | Crear nueva publicación                   | Sí            |
| POST   | `/comment`        | Agregar comentario                        | Sí            |
| POST   | `/delete-comment` | Eliminar comentario (solo autor)          | Sí            |
| POST   | `/confirm`        | Ejecutar eliminación de post (solo autor) | Sí            |
| POST   | `/auth`           | Iniciar sesión / registrar cuenta         | No            |
| GET    | `/logout`         | Cerrar sesión                             | No            |
| POST   | `/profile`        | Editar perfil (nombre, descripción)       | Sí            |
| POST   | `/save`           | Guardar post como favorito                | Sí            |
| POST   | `/unsave`         | Quitar post de favoritos                  | Sí            |

\* La confirmación requiere autenticación para ejecutar la eliminación.

## Modelo de datos

### Post

| Campo   | Tipo   | Descripción               |
|---------|--------|---------------------------|
| ID      | int    | Identificador único       |
| Title   | string | Título de la publicación  |
| User    | string | Nombre del autor          |
| Message | string | Contenido del post        |
| Time    | string | Fecha y hora de publicación (YYYY-MM-DD HH:MM) |

### Comment

| Campo    | Tipo   | Descripción                              |
|----------|--------|------------------------------------------|
| ID       | int    | Identificador único                      |
| PostID   | int    | ID del post al que pertenece             |
| ParentID | int    | ID del comentario padre (0 = raíz)       |
| User     | string | Nombre del autor                         |
| Message  | string | Contenido (o `[eliminado]` si borrado)   |
| Time     | string | Fecha y hora de publicación (YYYY-MM-DD HH:MM) |
| Deleted  | bool   | Indica si el comentario fue eliminado    |

### User

| Campo       | Tipo   | Descripción                          |
|-------------|--------|--------------------------------------|
| Username    | string | Nombre de usuario (clave en el mapa) |
| Password    | string | Hash bcrypt de la contraseña         |
| Description | string | Descripción del perfil               |
| SavedPostIDs| []int  | IDs de posts guardados como favoritos |

## Seguridad

- **Contraseñas** hasheadas con bcrypt (coste por defecto).
- **Autoría verificada en backend** — Tanto la eliminación de posts como de comentarios verifica que el usuario autenticado sea el autor.
- **Confirmación de título** — Para eliminar un post, el usuario debe escribir el título exacto, evitando eliminaciones accidentales.
- **Sesiones por cookie** — Nombre de cookie configurable, identificador único por sesión, sin exposición de contraseñas. Las sesiones pueden expirar automáticamente.
- **Validación de entrada** — Títulos y mensajes no vacíos, nombres de usuario únicos, etc.
- **Rate limiting** — Configurable vía `config.json` para evitar abusos.
- **HTTPS** — Soporte nativo configurable vía `config.json`.
- **SQLite** — Base de datos embebida con WAL mode para mejor concurrencia.

## Podado de comentarios

Cuando un usuario elimina un comentario, el sistema aplica podado ascendente automático:

1. El comentario se marca como `Deleted` y su mensaje se reemplaza por `[eliminado]`.
2. Si el comentario tiene hijos con contenido real (no eliminados), se conserva como marcador `[eliminado]`.
3. Si todos los descendientes del comentario están eliminados, se elimina por completo del árbol.
4. El proceso se repite hacia arriba: si el padre también está eliminado y todos sus hijos están muertos, se elimina también.

Esto evita que el árbol de comentarios se llene de `[eliminado]` innecesarios.

## Limitaciones

- **Sin migraciones** — La base de datos se crea desde cero si no existe; no hay sistema de migraciones.
- **Sin roles** — Todos los usuarios tienen el mismo nivel de permisos.

## Licencia

GPLv2 — Ver el archivo [LICENSE](LICENSE) para más detalles.
