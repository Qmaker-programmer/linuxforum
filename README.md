# Linux Forum

Linux Forum es un sistema de foros ligero, rápido y fácil de integrar en cualquier sitio web. Usa SQLite, sesiones por cookie, configuración vía JSON, y cuenta con un sistema de comentarios anidados con podado inteligente. 
Sin JavaScript, sin frameworks, solo Go puro

## Características

- **Publicaciones** — Creación, edición, visualización, eliminación (solo autor, con confirmación del título) y filtrado por fecha/título
- **Historial de versiones y revert** — Cada edición de un post guarda una copia de cómo estaba antes; el autor puede ver ese historial completo (`/post-history?id=N`) y revertir a cualquier versión anterior, lo cual a su vez queda registrado como una versión más (revertir también es reversible). Sin poda automática todavía — las versiones se acumulan sin límite mientras el post exista, y se limpian recién si se borra el post entero
- **Markdown** — Posts y comentarios se redactan en Markdown (GFM) y se renderizan sanitizados; hay previsualización antes de publicar
- **Imágenes** — Se pueden insertar imágenes (PNG/JPEG/GIF/WEBP, hasta 5 MB) al redactar un post o comentario; el tipo real del archivo se valida por contenido, no por extensión
- **Borradores** — Tanto de posts nuevos, de ediciones a un post existente, como de comentarios: se guardan, se listan por separado en `/drafts` (un borrador de edición se distingue como "Editando: <título>") y se retoman más tarde sin perder lo escrito
- **Paginación** — Los listados de posts (inicio, filtrado, búsqueda) se paginan de a 20
- **Filtrado** — Ordenar posts por fecha (asc/desc) o título (A-Z / Z-A) desde la página principal y desde los resultados de búsqueda
- **Comentarios anidados** — Respuestas en árbol con profundidad arbitraria
- **Podado inteligente** — Al eliminar un comentario, si todo su subárbol está muerto (solo `[eliminado]`), se elimina por completo, incluyendo ancestros muertos
- **Autenticación** — Registro e inicio de sesión con contraseñas hasheadas (bcrypt); con correo configurado, el registro requiere activación por email
- **Sesiones** — Persistidas en SQLite, cookie configurable con soporte de expiración y limpieza automática de sesiones vencidas
- **Guardado de posts** — Marca posts como favoritos (solo visibles para el usuario)
- **Búsquedas** — Búsqueda de publicaciones por título, búsqueda de usuarios por nombre (coincidencia parcial), búsqueda en comentarios
- **Perfiles** — Perfil de usuario con descripción editable, correo y cambio de nombre de usuario
- **Recuperación de cuenta por correo** — Reset de contraseña y eliminación de cuenta/post vía enlace de confirmación (si hay correo configurado)
- **Rate limiting** — Configurable por JSON: máximo de requests por ventana de tiempo
- **HTTPS** — Soporte nativo configurable vía JSON
- **Todo en backend** — Sin JavaScript, solo formularios HTML y redirecciones del servidor
- **Dark mode** — Conmutable desde la upbar sin JS, vía cookie y CSS class, respeta la preferencia del sistema
- **SQLite** — Base de datos persistente con AUTOINCREMENT y WAL mode
- **Migraciones** — Sistema de migraciones progresivas con control de versiones

## Stack

- **Lenguaje:** Go 1.25.12+ (versión fijada en `go.mod` — trae parches de seguridad de la librería estándar; verificado con `govulncheck`)
- **Dependencias:** `golang.org/x/crypto` (bcrypt), `github.com/mattn/go-sqlite3`, `github.com/yuin/goldmark` (Markdown) y `github.com/microcosm-cc/bluemonday` (sanitización de HTML)
- **Frontend:** HTML templates (`html/template`), CSS plano (`style.css`) sin JavaScript ni frameworks
- **Base de datos:** SQLite con WAL mode

## Instalación y uso

```bash
go run ./src
```

El servidor corre en `http://localhost:8080` (puerto configurable).

## Integración en tu sitio web

¿Quieres agregar un foro a tu sitio existente? ¡Es súper fácil! Aquí te explico cómo:

### Opción 1: Como subdominio (recomendado)

1. **Configura el subdominio** en tu DNS (ej: `foro.tusitio.com`)
2. **Ejecuta Linux Forum** en un puerto específico (ej: 8080)
3. **Configura tu reverse proxy** (nginx, Apache, Caddy) para redirigir el subdominio al puerto de Linux Forum

**Ejemplo con nginx:**
```nginx
server {
    listen 80;
    server_name foro.tusitio.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### Opción 2: Como subdirectorio

1. **Ejecuta Linux Forum** en un puerto (ej: 8080)
2. **Configura tu reverse proxy** para redirigir `/foro` al puerto de Linux Forum

**Ejemplo con nginx:**
```nginx
location /foro/ {
    proxy_pass http://localhost:8080/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

### Opción 3: Como servicio separado

Si usas Docker o systemd, puedes ejecutar Linux Forum como un servicio independiente y conectarlo a tu sitio web mediante proxy.

**Ejemplo con Docker:**
```dockerfile
FROM golang:1.25
WORKDIR /app
COPY . .
RUN go build -o linuxforum ./src
CMD ["./linuxforum"]
```

**Ejemplo con systemd:**
```ini
[Unit]
Description=Linux Forum
After=network.target

[Service]
Type=simple
User=forum
WorkingDirectory=/var/www/linuxforum
ExecStart=/usr/local/bin/linuxforum
Restart=always

[Install]
WantedBy=multi-user.target
```

> [!TIP]
> `systemctl stop` envía SIGTERM — Linux Forum lo captura y hace un apagado ordenado (deja de aceptar conexiones nuevas, espera a que terminen las que están en curso), así que no hace falta `KillMode`/`TimeoutStopSec` especiales.

Si sigues cualquiera de las tres opciones detrás de un reverse proxy (todas lo son, salvo que expongas el puerto de Linux Forum directamente a internet), activa `trust_proxy_headers` en `config.json` para que el rate limiting funcione por visitante y no por el total del tráfico del sitio — ver la tabla de configuración más abajo.

### Personalización del diseño

Puedes personalizar el diseño editando los archivos en `web/`:
- `style.css` — Estilos generales
- `head.html` — Meta tags y enlaces CSS
- `upbar.html` — Barra de navegación superior
- `*.html` — Plantillas de cada página

### Base de datos compartida

Si quieres compartir la base de datos con tu aplicación principal:
1. Configura `db_path` en `config.json` para apuntar a tu base de datos SQLite existente
2. Asegúrate de que tu aplicación no interfiera con las tablas de Linux Forum

### Autenticación compartida

Si ya tienes un sistema de autenticación, puedes:
1. Modificar `getSession()` en `templates.go` para validar tokens de tu sistema
2. O usar Linux Forum como sistema de autenticación independiente

## Configuración (`config.json`)

| Campo | Descripción | Default |
|---|---|---|
| `rate_limit` | Máximo de solicitudes por ventana de tiempo | `100` |
| `reset_minutes` | Minutos para reiniciar el contador de rate limit | `1` |
| `port` | Puerto del servidor | `8080` |
| `db_path` | Ruta al archivo de base de datos SQLite | `db/forum.db` |
| `https` | Habilitar HTTPS | `false` |
| `cert_file` | Ruta al certificado SSL | `cert.pem` |
| `key_file` | Ruta a la llave SSL | `key.pem` |
| `session_token_name` | Nombre de la cookie de sesión | `session_token` |
| `session_expire_minutes` | Minutos hasta expirar la sesión (0 = nunca) | `0` |
| `trust_proxy_headers` | Usar `X-Forwarded-For`/`X-Real-IP` para el rate limiting en vez de la IP de conexión | `false` |
| `backup_interval_hours` | Horas entre backups automáticos de la base de datos | `120` (5 días) |
| `max_backups` | Cuántos backups conservar antes de podar los más viejos (0 = nunca podar) | `0` |
| `log_level` | Verbosidad del log: `debug`, `info`, `warn` o `error` | `info` |

> [!WARNING]
> Solo activa `trust_proxy_headers` si el servidor **únicamente** recibe tráfico a través de tu reverse proxy (nginx/Apache/Caddy) y este siempre sobrescribe esos headers. Si el puerto de Linux Forum queda accesible directamente además del proxy, cualquiera puede falsificar `X-Forwarded-For` para saltarse el rate limiting o hacer que se banee la IP de otra persona. Con la configuración por defecto (`false`), el rate limiting se basa en la IP con la que el proxy se conecta a Linux Forum — si corres detrás de un proxy sin activar esta opción, todo el tráfico del sitio comparte un mismo cupo.

## Configuración de correo (`noUpload/mail.json`)

Para habilitar la recuperación de contraseña por correo, crea el archivo `noUpload/mail.json` con las credenciales SMTP:

```json
{
    "mail": "tu-correo@ejemplo.com",
    "password": "tu-contraseña-de-aplicación",
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587
}
```

> [!WARNING]
> Este archivo contiene credenciales sensibles y está en `.gitignore`. No se sube al repositorio.

### Campos

| Campo | Descripción | Default |
|---|---|---|
| `mail` | Correo electrónico desde el que se enviarán los enlaces de recuperación | — |
| `password` | Contraseña de aplicación (para Gmail, genera una en https://myaccount.google.com/apppasswords) | — |
| `smtp_host` | Servidor SMTP (se autodetecta para gmail.com, outlook.com, hotmail.com, yahoo.com) | Autodetectado |
| `smtp_port` | Puerto SMTP (587 con STARTTLS) | `587` |

### Seguridad

- Usa siempre una **contraseña de aplicación**, nunca tu contraseña principal
- El servidor SMTP debe soportar STARTTLS (puerto 587 estándar)
- Los tokens de recuperación:
  - Se generan con `crypto/rand` (32 bytes → 64 caracteres hex)
  - Se almacenan hasheados con SHA-256 en la base de datos (nunca en texto plano)
  - Expiran a la **1 hora** de su creación
  - Son de **un solo uso** (se eliminan al usarlos)
  - Se limpian automáticamente cada 30 minutos
- No se revela si un correo está registrado o no (previene enumeración de cuentas)

## Logging

Toda la salida pasa por [`log/slog`](https://pkg.go.dev/log/slog) de la librería estándar (nada de dependencias nuevas), en texto plano con nivel y timestamp por línea, por ejemplo:

```
time=2026-07-16T18:30:00.000-06:00 level=INFO msg="Servidor corriendo" addr=http://localhost:8080
time=2026-07-16T18:30:05.123-06:00 level=ERROR msg="No se pudo enviar el correo de reset de contraseña" err="..."
```

- `log_level` en `config.json` controla la verbosidad (`debug`/`info`/`warn`/`error`, default `info`)
- Se escribe a stdout — con `systemd`, `journalctl -u linuxforum` ya lo captura y lo deja filtrable por fecha/nivel sin configuración extra
- No hay envío a un servicio externo (Sentry, etc.) ni rotación de archivos propia — si necesitás eso, se resuelve a nivel de infraestructura (systemd/journald ya rota, o redirigí stdout a tu colector de logs preferido)

## Backups

Desde que arranca, Linux Forum guarda automáticamente una copia de la base de datos cada `backup_interval_hours` horas (5 días por defecto) en `db/backups/forum-YYYYMMDD-HHMMSS.db`, y deja constancia en el log (`Backup de la base de datos creado`).

- Usa `VACUUM INTO`, que genera una copia consistente aunque el servidor siga leyendo y escribiendo al mismo tiempo — a diferencia de copiar el archivo `.db` a mano, no se arriesga a perderse cambios que todavía están solo en el WAL (`forum.db-wal`)
- El conteo de horas arranca de nuevo en cada reinicio del proceso (igual que el resto de las tareas periódicas de este proyecto); no se persiste "cuándo tocaría el próximo backup"
- **Poda automática opcional** — con `max_backups` en `config.json` (0 = nunca podar, el default), tras cada backup exitoso se conservan solo los `max_backups` más recientes y se borran los demás, registrando cada eliminación en el log
- Para probar sin esperar días, bajá `backup_interval_hours` a `1` (o menos, en un entorno de prueba) y reiniciá el servidor

## Estructura del proyecto

```
linuxforum/
├── config.json          # Configuración general del servidor
├── .gitignore
├── db/                  # Base de datos SQLite
│   └── backups/         # Backups automáticos (gitignored)
├── go.mod               # Módulo Go
├── go.sum               # Checksum de dependencias
├── README.md
├── LICENSE
├── src/                 # Código fuente Go (package main)
│   ├── main.go          # Entry point, configuración, rate limiting
│   ├── types.go         # Structs y variables globales
│   ├── db.go            # Operaciones de base de datos
│   ├── templates.go     # Renderizado de templates, paginación y helpers
│   ├── handlers.go      # Todos los HTTP handlers
│   ├── markdown.go      # Render de Markdown (goldmark) y sanitización (bluemonday)
│   ├── uploads.go       # Subida/validación/limpieza de imágenes
│   ├── backup.go        # Backups periódicos de la base de datos (VACUUM INTO)
│   ├── logging.go       # Configuración de log/slog (nivel dinámico)
│   └── mail.go          # SMTP y flujos de activación/reset/eliminación por correo
└── web/
    ├── head.html            # Template <head> compartido
    ├── upbar.html           # Barra superior de navegación
    ├── index.html           # Página principal (lista de posts + filtros + paginación)
    ├── filtered.html        # Resultados de filtrado (paginados)
    ├── post.html            # Vista de post + comentarios
    ├── search.html          # Resultados de búsqueda + filtros (paginados)
    ├── user.html            # Perfil de usuario
    ├── login.html           # Inicio de sesión / registro
    ├── public.html          # Formulario de nuevo post
    ├── edit_post.html       # Editor de post (Markdown, imágenes, borrador)
    ├── post_preview.html    # Previsualización de post antes de publicar
    ├── comment.html         # Editor de comentario (Markdown, imágenes, borrador)
    ├── comment_preview.html # Previsualización de comentario antes de publicar
    ├── drafts.html          # Borradores de posts (nuevos o de ediciones) y de comentarios
    ├── post-history.html    # Historial de versiones de un post + revert
    ├── confirm.html         # Confirmación de eliminación de post
    ├── confirm-post-deletion.html  # Confirmación de eliminación de post por correo
    ├── confirm-deletion.html      # Confirmación de eliminación de cuenta por correo
    ├── forgot.html          # Solicitud de recuperación de contraseña
    ├── reset.html           # Formulario de cambio de contraseña
    ├── style.css            # Estilos visuales
    ├── uploads/             # Imágenes subidas por los usuarios (gitignored)
    └── tux.png              # Mascota Tux
```

## Rutas de la API

| Método | Ruta                       | Descripción                                            | Autenticación |
|--------|----------------------------|---------------------------------------------------------|---------------|
| GET    | `/?page=N`                 | Lista publicaciones (paginado, 20 por página)            | No            |
| GET    | `/filtered?page=N`         | Lista publicaciones ordenadas (paginado)                 | No            |
| GET    | `/view?id=N`               | Ver un post y sus comentarios                            | No            |
| GET    | `/search?query=X&page=N`   | Buscar posts por título (paginado)                       | No            |
| GET    | `/search?user=X`           | Buscar usuarios por nombre                               | No            |
| GET    | `/user?u=X`                | Ver perfil de usuario                                    | No            |
| GET    | `/confirm?id=N`            | Página de confirmación para eliminar post                | No*           |
| POST   | `/post`                    | Crear o editar post, previsualizar, insertar imagen      | Sí            |
| GET    | `/post-form?id=N`           | Formulario de nuevo post, editar uno existente (autor) o retomar un borrador | Sí |
| GET    | `/post-history?id=N`       | Ver historial de versiones de un post (autor)            | Sí            |
| POST   | `/post-revert`             | Revertir un post a una versión anterior (autor)          | Sí            |
| POST   | `/draft`                   | Guardar borrador de post (nuevo o de una edición)        | Sí            |
| GET    | `/drafts`                  | Listar borradores de posts y de comentarios              | Sí            |
| POST   | `/draft-delete`            | Eliminar borrador de post                                | Sí            |
| POST   | `/comment`                 | Agregar comentario, previsualizar, insertar imagen o editar | Sí         |
| GET    | `/comment-form`             | Formulario de comentario (o retomar un borrador)         | Sí            |
| POST   | `/comment-draft`           | Guardar borrador de comentario                           | Sí            |
| POST   | `/comment-draft-delete`    | Eliminar borrador de comentario                          | Sí            |
| POST   | `/delete-comment`          | Eliminar comentario (solo autor)                         | Sí            |
| POST   | `/confirm`                 | Ejecutar eliminación de post (solo autor)                | Sí            |
| POST   | `/auth`                    | Iniciar sesión / registrar cuenta                        | No            |
| GET    | `/activate?token=X`        | Activar cuenta tras registro por correo                  | No (token)    |
| GET    | `/theme?mode=X`            | Cambiar modo oscuro/claro vía cookie                     | No            |
| GET    | `/logout`                  | Cerrar sesión                                            | No            |
| POST   | `/profile`                 | Editar perfil (nombre, descripción, correo)              | Sí            |
| POST   | `/save`                    | Guardar post como favorito                               | Sí            |
| POST   | `/unsave`                  | Quitar post de favoritos                                 | Sí            |
| GET    | `/forgot`                  | Formulario de recuperación de contraseña                 | No            |
| POST   | `/forgot`                  | Enviar enlace de recuperación por correo                 | No            |
| GET    | `/reset?token=X`           | Formulario para cambiar contraseña                       | No (token)    |
| POST   | `/reset`                   | Ejecutar cambio de contraseña                            | No (token)    |
| POST   | `/request-delete`          | Solicitar eliminación de cuenta por correo               | Sí            |
| GET    | `/confirm-deletion?token=X`| Confirmar eliminación de cuenta                          | No (token)    |
| GET    | `/confirm-post-deletion?token=X` | Confirmar eliminación de post por correo           | No (token)    |

\* La confirmación requiere autenticación para ejecutar la eliminación.

## Modelo de datos

### Post

| Campo     | Tipo         | Descripción               |
|-----------|--------------|---------------------------|
| ID        | int          | Identificador único       |
| Title     | string       | Título de la publicación  |
| User      | string       | Nombre del autor          |
| Message   | string       | Contenido en Markdown crudo |
| Markdown  | template.HTML| Contenido ya renderizado y sanitizado |
| Time      | string       | Fecha y hora de publicación (YYYY-MM-DD HH:MM) |
| UpdatedAt | string       | Fecha de la última edición (vacío si nunca se editó) |

### PostRevision (versión anterior de un post)

| Campo    | Tipo         | Descripción                                    |
|----------|--------------|-------------------------------------------------|
| ID       | int          | Identificador único                              |
| PostID   | int          | Post al que pertenece                            |
| Title    | string       | Título como estaba antes de ese cambio           |
| Message  | string       | Contenido en Markdown como estaba antes          |
| Markdown | template.HTML| Contenido renderizado como estaba antes          |
| EditedAt | string       | Momento en que se reemplazó por la versión siguiente |

### Comment

| Campo    | Tipo         | Descripción                              |
|----------|--------------|------------------------------------------|
| ID       | int          | Identificador único                      |
| PostID   | int          | ID del post al que pertenece             |
| ParentID | int          | ID del comentario padre (0 = raíz)       |
| User     | string       | Nombre del autor                         |
| Message  | string       | Contenido en Markdown (o `[eliminado]` si borrado) |
| Markdown | template.HTML| Contenido ya renderizado y sanitizado    |
| Time     | string       | Fecha y hora de publicación (YYYY-MM-DD HH:MM) |
| Deleted  | bool         | Indica si el comentario fue eliminado    |

### Draft (borrador de post)

| Campo            | Tipo   | Descripción                    |
|------------------|--------|---------------------------------|
| ID               | int    | Identificador único             |
| Username         | string | Dueño del borrador               |
| Title            | string | Título en progreso               |
| Message          | string | Contenido en progreso (Markdown) |
| EditingPostID    | int    | 0 = borrador de post nuevo; si no, ID del post existente que se está editando |
| EditingPostTitle | string | Título actual de ese post (para mostrarlo en `/drafts`; vacío si el post ya no existe) |
| CreatedAt        | string | Fecha de creación                |
| UpdatedAt        | string | Fecha de última edición          |

### CommentDraft (borrador de comentario)

| Campo     | Tipo   | Descripción                                  |
|-----------|--------|------------------------------------------------|
| ID        | int    | Identificador único                             |
| Username  | string | Dueño del borrador                              |
| PostID    | int    | Post al que respondería el comentario           |
| ParentID  | int    | Comentario padre (0 = raíz)                     |
| Message   | string | Contenido en progreso (Markdown)                |
| PostTitle | string | Título del post (para mostrarlo en `/drafts`; vacío si el post ya no existe) |
| CreatedAt | string | Fecha de creación                               |
| UpdatedAt | string | Fecha de última edición                         |

### User

| Campo       | Tipo   | Descripción                          |
|-------------|--------|--------------------------------------|
| Username    | string | Nombre de usuario (clave en el mapa) |
| Password    | string | Hash bcrypt de la contraseña         |
| Description | string | Descripción del perfil               |
| Email       | string | Correo del usuario (opcional)        |
| SavedPostIDs| []int  | IDs de posts guardados como favoritos |

## Seguridad

- **Contraseñas** hasheadas con bcrypt (coste por defecto)
- **Autoría verificada en backend** — Tanto la eliminación de posts como de comentarios verifica que el usuario autenticado sea el autor
- **Confirmación de título** — Para eliminar un post, el usuario debe escribir el título exacto, evitando eliminaciones accidentales
- **Sesiones por cookie** — Nombre de cookie configurable, identificador único por sesión, sin exposición de contraseñas. Las sesiones pueden expirar automáticamente
- **Tokens de sesión hasheados** — Igual que los de reset/activación/eliminación, la base de datos solo guarda el hash SHA-256 del token de sesión, nunca el valor de la cookie en sí; leer la base de datos a solas no alcanza para secuestrar una sesión
- **Bloqueo por intentos fallidos de login** — Tras 5 intentos fallidos, ese nombre de usuario queda bloqueado 15 minutos (aunque después se use la contraseña correcta), para frenar fuerza bruta
- **Validación de entrada** — Títulos y mensajes no vacíos, nombres de usuario únicos, etc
- **Rate limiting** — Configurable vía `config.json` para evitar abusos; por IP de conexión, o por `X-Forwarded-For`/`X-Real-IP` si se activa `trust_proxy_headers` (ver arriba)
- **HTTPS** — Soporte nativo configurable vía `config.json`
- **Headers de seguridad** — `Content-Security-Policy` (sin JS de terceros; el único script inline está anclado por hash), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy` en toda respuesta
- **Apagado ordenado** — Ante SIGTERM/SIGINT (ej. `systemctl stop`), el servidor deja de aceptar conexiones nuevas y espera hasta 10s a que las que están en curso terminen antes de cerrar la base de datos
- **SQLite** — Base de datos embebida con WAL mode para mejor concurrencia
- **Markdown sanitizado** — Se renderiza con goldmark y se sanitiza con bluemonday antes de guardarse como HTML
- **Imágenes validadas por contenido** — El tipo de archivo se determina inspeccionando los bytes reales (no la extensión ni el `Content-Type` del cliente); se limita a 5 MB y a PNG/JPEG/GIF/WEBP (sin SVG, para evitar XSS)
- **Limpieza de huérfanos** — Al eliminar un borrador, comentario, post o cuenta, las imágenes subidas y los borradores asociados se eliminan también, no quedan archivos ni filas huérfanas

## Podado de comentarios

Cuando un usuario elimina un comentario, el sistema aplica podado ascendente automático:

1. El comentario se marca como `Deleted` y su mensaje se reemplaza por `[eliminado]`.
2. Si el comentario tiene hijos con contenido real (no eliminados), se conserva como marcador `[eliminado]`.
3. Si todos los descendientes del comentario están eliminados, se elimina por completo del árbol.
4. El proceso se repite hacia arriba: si el padre también está eliminado y todos sus hijos están muertos, se elimina también.

Esto evita que el árbol de comentarios se llene de `[eliminado]` innecesarios.

## Filosofía

- **Sin roles** — Todos los usuarios tienen el mismo nivel de permisos. No hay administradores ni moderadores: cero riesgo de compromiso de cuenta privilegiada

## Licencia

AGPLv3 — Ver el archivo [LICENSE](LICENSE) para más detalles.

---

## Contribuciones

¡Las contribuciones son bienvenidas! Si encuentras un bug o tienes una idea para mejorar Linux Forum, siéntete libre de abrir un issue o enviar un pull request.

## Soporte

¿Necesitas ayuda? Abre un issue en el repositorio y te responderemos lo antes posible.

---

**¡Hecho con Go!**
