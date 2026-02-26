# Process Browser - Especificación de Diseño

## Resumen

App de monitoreo de sistema estilo btop pero con interfaz web moderna tipo Grafana. Escrita en Go con frontend Vue 3. Permite monitoreo local y remoto.

## Modos de Ejecución

```bash
# Modo local: abre browser automáticamente
process-browser

# Modo servidor: imprime URL, no abre browser
process-browser --serve

# Con archivo de configuración
process-browser --serve --config-file=config.json

# Emitir config file con valores por defecto (para personalizar)
process-browser --print-config-file > config.json
```

## Arquitectura

```
┌─────────────────────────────────────────────────────────┐
│                    process-browser                       │
├─────────────────────────────────────────────────────────┤
│  Go Backend                                              │
│  ├── main.go           CLI, config, server setup        │
│  ├── collectors/       CPU, mem, disk, net, GPU, proc   │
│  ├── api/              REST endpoints + SSE stream      │
│  └── auth/             middleware, session              │
├─────────────────────────────────────────────────────────┤
│  Frontend (Vue 3 SPA)                                    │
│  ├── static/app.js     Vue app en módulos ES6           │
│  ├── static/style.css  Estilos                          │
│  └── templates/index.html  Shell que carga Vue          │
└─────────────────────────────────────────────────────────┘
```

**Distribución:** Todo embebido en el binario con `go:embed` para distribuir un solo archivo ejecutable.

## Paneles del Dashboard

### CPU
- Uso por core (barras o gráficos)
- Temperaturas por core
- Frecuencia actual
- Load average

### Memoria
- Total / Usado / Disponible
- Cached / Buffers / Free
- Swap usado/total
- Barras de progreso visuales

### Discos
- Uso por partición (%, usado, total)
- Velocidad de I/O (lectura/escritura)

### Red
- Interfaces de red activas
- Velocidad download/upload actual
- Totales de tráfico
- IP de cada interfaz

### GPU
- Uso de GPU (%)
- Temperatura
- Memoria usada/total
- Soporte para nvidia-smi, y extensible a AMD

### Procesos
- Tabla con columnas: PID, nombre, comando, threads, usuario, memoria, CPU%
- Ordenable por cualquier columna
- Filtro/búsqueda
- Click en proceso abre panel de detalles

### Sockets Abiertos
- Lista de conexiones TCP/UDP
- Estado (LISTEN, ESTABLISHED, etc.)
- Proceso asociado
- Direcciones local y remota

### Firewall
- Puertos abiertos según firewall activo
- Soporte para: iptables, nftables, ufw, firewalld
- Detección automática del firewall en uso

## Detalle de Proceso

Al clickear un proceso, mostrar panel con información extrema:

- PID, PPID (proceso padre)
- Usuario y grupo
- Estado (running, sleeping, zombie, etc.)
- Uptime del proceso
- Comando completo con argumentos
- Working directory
- Variables de entorno
- File descriptors abiertos
- Puertos que está usando
- Conexiones de red activas
- Uso de CPU histórico (mini gráfico)
- Uso de memoria histórico (mini gráfico)
- Threads del proceso
- Árbol de procesos hijos

## Acciones sobre Procesos (requiere autenticación)

- **Kill** - Enviar señal (SIGTERM, SIGKILL, otras)
- **Renice** - Cambiar prioridad
- **Detach** - Desasociar de proceso padre (reparent a init)

## Autenticación

**Modo sin config o sin credenciales:**
- App funciona en modo solo lectura
- Se puede ver todo pero no ejecutar acciones

**Modo con credenciales en config.json:**
- Login con usuario/contraseña
- Session cookie
- Acciones destructivas habilitadas
- Control de permisos también en backend (no solo UI)

```json
{
  "auth": {
    "username": "admin",
    "password": "secreto"
  }
}
```

## Configuración (config.json)

Ejecutar `process-browser --print-config-file` genera un JSON con todos los valores por defecto, listo para personalizar:

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "ssl": {
      "enabled": false,
      "cert": "",
      "key": ""
    }
  },
  "auth": {
    "username": "",
    "password": ""
  },
  "ui": {
    "title": "Process Browser",
    "headerColor": "#1a1a2e",
    "favicon": "",
    "theme": "dark",
    "compactMode": false
  },
  "refresh": {
    "cpu": 5000,
    "memory": 5000,
    "disk": 5000,
    "network": 5000,
    "gpu": 5000,
    "processes": 5000,
    "sockets": 5000,
    "firewall": 10000
  }
}
```

### Opciones de UI

| Campo | Descripción | Default |
|-------|-------------|---------|
| `title` | Título en el browser tab y header | "Process Browser" |
| `headerColor` | Color del header (hex, rgb, nombre CSS) | "#1a1a2e" |
| `favicon` | Path a favicon custom, o vacío para usar el default | "" |
| `theme` | Tema inicial: "dark" o "light" | "dark" |
| `compactMode` | Iniciar en modo compacto | false |

Esto permite diferenciar dashboards visualmente cuando tenés varios servidores monitoreados:

```json
// Server de producción - rojo
{ "ui": { "title": "PROD Server", "headerColor": "#8b0000" } }

// Server de staging - amarillo
{ "ui": { "title": "Staging", "headerColor": "#b8860b" } }

// Server de dev - verde
{ "ui": { "title": "Dev Local", "headerColor": "#006400" } }
```

## Actualización en Tiempo Real

**Tecnología:** Server-Sent Events (SSE)

**Endpoint:** `GET /api/stream`

**Frecuencias por defecto:** 5 segundos para todo, configurable por sección.

**Controles de pausa:**
- Botón pause en cada sección (pausa solo esa sección)
- Botón pause general en el dashboard (pausa todo)

Permite leer datos sin que salten mientras se está analizando.

## Diseño Visual

**Estilo:** Dashboard moderno tipo Grafana

**Temas:**
- Dark mode (por defecto)
- Light mode
- Toggle en UI para cambiar

**Densidad:**
- Modo normal
- Modo compacto (mismas secciones, elementos más pequeños)
- Toggle en UI para cambiar

**Características:**
- Cards con bordes redondeados
- Sombras suaves
- Gráficos con colores vivos
- Layout responsive
- Barras de progreso para porcentajes

## API REST

### Endpoints de Datos (públicos, solo lectura)

```
GET /api/cpu          - Datos de CPU
GET /api/memory       - Datos de memoria
GET /api/disk         - Datos de discos
GET /api/network      - Datos de red
GET /api/gpu          - Datos de GPU
GET /api/processes    - Lista de procesos
GET /api/process/:pid - Detalle de un proceso
GET /api/sockets      - Sockets abiertos
GET /api/firewall     - Reglas de firewall
GET /api/stream       - SSE stream con todos los datos
```

### Endpoints de Acciones (requieren autenticación)

```
POST /api/auth/login   - Login
POST /api/auth/logout  - Logout
GET  /api/auth/status  - Estado de sesión

POST /api/process/:pid/kill    - Matar proceso
POST /api/process/:pid/renice  - Cambiar prioridad
POST /api/process/:pid/detach  - Desasociar de parent
```

## Stack Tecnológico

**Backend:**
- Go 1.21+
- Librería estándar para HTTP
- go:embed para embeber frontend
- No dependencias externas pesadas

**Frontend:**
- Vue 3 (via CDN o embebido)
- CSS puro o Tailwind (por decidir)
- Chart.js o similar para gráficos
- Sin build step complejo (ES modules directos)

## Estructura de Archivos

```
process-browser/
├── main.go
├── go.mod
├── config.example.json
├── build                    # Script de build
├── run                      # Script de desarrollo
├── specs/
│   └── design.md           # Este documento
├── collectors/
│   ├── cpu.go
│   ├── memory.go
│   ├── disk.go
│   ├── network.go
│   ├── gpu.go
│   ├── process.go
│   ├── sockets.go
│   └── firewall.go
├── api/
│   ├── routes.go
│   ├── handlers.go
│   └── sse.go
├── auth/
│   ├── middleware.go
│   └── session.go
├── static/
│   ├── app.js
│   ├── components/         # Componentes Vue
│   └── style.css
└── templates/
    └── index.html
```

## Fases de Implementación

### Fase 1 - MVP
- Estructura básica Go
- Collectors de CPU, memoria, procesos
- SSE streaming
- UI básica con Vue 3
- Lista de procesos funcional

### Fase 2 - Completar Paneles
- Collectors de disco, red, GPU
- Sockets y firewall
- Todos los paneles en UI
- Detalle de proceso completo

### Fase 3 - Autenticación y Acciones
- Sistema de auth
- Acciones sobre procesos (kill, renice, detach)
- Configuración via JSON
- SSL support

### Fase 4 - Polish
- Dark/light mode
- Modo compacto
- Configuración de refresh rates
- Pausar secciones/dashboard
- Responsive design
