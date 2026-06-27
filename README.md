# Linux Forum

Un foro minimalista y autónomo escrito en Go, con almacenamiento en memoria, sesiones por cookie, y un sistema de comentarios anidados con podado inteligente.

## Características

- **Publicaciones** — Creación, visualización y eliminación (solo autor, con confirmación del título).
- **Comentarios anidados** — Respuestas en árbol con profundidad arbitraria.
- **Podado inteligente** — Al eliminar un comentario, si todo su subárbol está muerto (solo `[eliminado]`), se elimina por completo, incluyendo ancestros muertos.
- **Autenticación** — Registro e inicio de sesión con contraseñas hasheadas (bcrypt, coste por defecto).
- **Sesiones** — Cookie `session_token` con identificador único (timestamp en nanosegundos).
- **Guardado de posts** — Marca posts como favoritos (solo visibles para el usuario).
- **Búsquedas** — Búsqueda de publicaciones por título, búsqueda de usuarios por nombre (coincidencia parcial), búsqueda en comentarios.
- **Perfiles** — Perfil de usuario con descripción editable y cambio de nombre de usuario.
- **Sin base de datos** — Todo en memoria, cero dependencias externas.

## Stack

- **Lenguaje:** Go 1.25+
- **Dependencias:** Solo `golang.org/x/crypto` para bcrypt.
- **Frontend:** HTML templates (`html/template`) sin JavaScript ni frameworks.
- **Estilo:** Sin CSS (sin archivo `style.css` creado aún).

## Instalación y uso

```bash
go run main.go
```

El servidor corre en `http://localhost:8080`.

### Cuenta de administración por defecto

| Usuario | Contraseña |
|---------|-----------|
| admin   | 1234      |

## Estructura del proyecto

```
linuxforum/
├── main.go              # Servidor completo (single file, ~950 líneas)
├── go.mod               # Módulo Go
├── go.sum               # Checksum de dependencias
├── README.md
├── LICENSE
└── web/
    ├── head.html        # Template <head> compartido
    ├── upbar.html       # Barra lateral de navegación
    ├── index.html       # Página principal (lista de posts)
    ├── post.html        # Vista de post + comentarios
    ├── search.html      # Resultados de búsqueda
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
| Time    | string | Hora de publicación (HH:MM) |

### Comment

| Campo    | Tipo   | Descripción                              |
|----------|--------|------------------------------------------|
| ID       | int    | Identificador único                      |
| PostID   | int    | ID del post al que pertenece             |
| ParentID | int    | ID del comentario padre (0 = raíz)       |
| User     | string | Nombre del autor                         |
| Message  | string | Contenido (o `[eliminado]` si borrado)   |
| Time     | string | Hora de publicación (HH:MM)              |
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
- **Sesiones por cookie** — Identificador único por sesión, sin exposición de contraseñas.
- **Validación de entrada** — Títulos y mensajes no vacíos, nombres de usuario únicos, etc.
- **Rate limiting** — Máximo 100 solicitudes por minuto por IP para evitar abusos.
- **HTTPS opcional** — Activable con la flag `-wh` (with https). Requiere `cert.pem` y `key.pem`.
- **Sin dependencias externas** — Solo bcrypt para hash de contraseñas, mínimo vector de ataque.

## Podado de comentarios

Cuando un usuario elimina un comentario, el sistema aplica podado ascendente automático:

1. El comentario se marca como `Deleted` y su mensaje se reemplaza por `[eliminado]`.
2. Si el comentario tiene hijos con contenido real (no eliminados), se conserva como marcador `[eliminado]`.
3. Si todos los descendientes del comentario están eliminados, se elimina por completo del árbol.
4. El proceso se repite hacia arriba: si el padre también está eliminado y todos sus hijos están muertos, se elimina también.

Esto evita que el árbol de comentarios se llene de `[eliminado]` innecesarios.

## Limitaciones

- **Almacenamiento en memoria** — Los datos se pierden al reiniciar el servidor.
- **Sin base de datos** — No hay persistencia ni migraciones.

## Licencia

GPLv2 — Ver el archivo [LICENSE](LICENSE) para más detalles.
