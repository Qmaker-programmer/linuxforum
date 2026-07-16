# Linux Forum

¡Un foro minimalista y autónomo escrito en Go!

Linux Forum es un sistema de foros ligero, rápido y fácil de integrar en cualquier sitio web. Usa SQLite, sesiones por cookie, configuración vía JSON, y cuenta con un sistema de comentarios anidados con podado inteligente. ¡Sin JavaScript, sin frameworks, solo Go puro!

## Características

- **Publicaciones** — Creación, visualización, eliminación (solo autor, con confirmación del título) y filtrado por fecha/título
- **Filtrado** — Ordenar posts por fecha (asc/desc) o título (A-Z / Z-A) desde la página principal y desde los resultados de búsqueda
- **Comentarios anidados** — Respuestas en árbol con profundidad arbitraria
- **Podado inteligente** — Al eliminar un comentario, si todo su subárbol está muerto (solo `[eliminado]`), se elimina por completo, incluyendo ancestros muertos
- **Autenticación** — Registro e inicio de sesión con contraseñas hasheadas (bcrypt, coste por defecto)
- **Sesiones** — Cookie configurable con soporte de expiración y limpieza automática de sesiones vencidas
- **Guardado de posts** — Marca posts como favoritos (solo visibles para el usuario)
- **Búsquedas** — Búsqueda de publicaciones por título, búsqueda de usuarios por nombre (coincidencia parcial), búsqueda en comentarios
- **Perfiles** — Perfil de usuario con descripción editable y cambio de nombre de usuario
- **Rate limiting** — Configurable por JSON: máximo de requests por ventana de tiempo
- **HTTPS** — Soporte nativo configurable vía JSON
- **Todo en backend** — Sin JavaScript, solo formularios HTML y redirecciones del servidor
- **Dark mode** — Conmutable desde la upbar sin JS, vía cookie y CSS class, respeta la preferencia del sistema
- **SQLite** — Base de datos persistente con AUTOINCREMENT y WAL mode
- **Migraciones** — Sistema de migraciones progresivas con control de versiones

## Stack

- **Lenguaje:** Go 1.25+
- **Dependencias:** `golang.org/x/crypto` (bcrypt) y `github.com/mattn/go-sqlite3`
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

## Estructura del proyecto

```
linuxforum/
├── config.json          # Configuración general del servidor
├── .gitignore
├── db/                  # Base de datos SQLite
├── go.mod               # Módulo Go
├── go.sum               # Checksum de dependencias
├── README.md
├── LICENSE
├── src/                 # Código fuente Go (package main)
│   ├── main.go          # Entry point, configuración, rate limiting
│   ├── types.go         # Structs y variables globales
│   ├── db.go            # Operaciones de base de datos
│   ├── templates.go     # Renderizado de templates y helpers
│   └── handlers.go      # Todos los HTTP handlers
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
    ├── style.css        # Estilos visuales
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
| GET    | `/theme?mode=X`   | Cambiar modo oscuro/claro vía cookie      | No            |
| GET    | `/logout`         | Cerrar sesión                             | No            |
| POST   | `/profile`        | Editar perfil (nombre, descripción)       | Sí            |
| POST   | `/save`           | Guardar post como favorito                | Sí            |
| POST   | `/unsave`         | Quitar post de favoritos                  | Sí            |
| GET    | `/forgot`         | Formulario de recuperación de contraseña  | No            |
| POST   | `/forgot`         | Enviar enlace de recuperación por correo  | No            |
| GET    | `/reset?token=X`  | Formulario para cambiar contraseña        | No (token)    |
| POST   | `/reset`          | Ejecutar cambio de contraseña             | No (token)    |

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

- **Contraseñas** hasheadas con bcrypt (coste por defecto)
- **Autoría verificada en backend** — Tanto la eliminación de posts como de comentarios verifica que el usuario autenticado sea el autor
- **Confirmación de título** — Para eliminar un post, el usuario debe escribir el título exacto, evitando eliminaciones accidentales
- **Sesiones por cookie** — Nombre de cookie configurable, identificador único por sesión, sin exposición de contraseñas. Las sesiones pueden expirar automáticamente
- **Validación de entrada** — Títulos y mensajes no vacíos, nombres de usuario únicos, etc
- **Rate limiting** — Configurable vía `config.json` para evitar abusos
- **HTTPS** — Soporte nativo configurable vía `config.json`
- **SQLite** — Base de datos embebida con WAL mode para mejor concurrencia

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

GPLv3 — Ver el archivo [LICENSE](LICENSE) para más detalles.

---

## Contribuciones

¡Las contribuciones son bienvenidas! Si encuentras un bug o tienes una idea para mejorar Linux Forum, siéntete libre de abrir un issue o enviar un pull request.

## Soporte

¿Necesitas ayuda? Abre un issue en el repositorio y te responderemos lo antes posible.

---

**¡Hecho con Go!**
