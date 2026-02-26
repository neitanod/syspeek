# Process Browser - Mejoras Futuras

## Fase 2: Collectors Adicionales

### Panel de Usuarios del Sistema
- Leer `/etc/passwd` para listar usuarios
- Mostrar información de cada usuario:
  - UID/GID
  - Home directory
  - Shell
  - Grupos a los que pertenece
- Último login (desde `/var/log/lastlog` o comando `lastlog`)
- Sesiones activas actuales (desde `who` o `/var/run/utmp`)
- Procesos corriendo por cada usuario
- Uso de disco por usuario (si es factible)

### Panel de Servicios Systemd
- Listar servicios (`systemctl list-units --type=service`)
- Estado de cada servicio (running, stopped, failed)
- Habilitar/deshabilitar servicios (con auth)
- Start/stop/restart servicios (con auth)
- Ver logs de servicio (`journalctl -u <service>`)

### Panel de Docker/Containers
- Listar containers (docker ps)
- Estado, imagen, puertos expuestos
- Uso de CPU/memoria por container
- Start/stop containers (con auth)
- Ver logs de container

### Panel de Cron Jobs
- Listar cron jobs del sistema (`/etc/crontab`, `/etc/cron.d/`)
- Listar cron jobs por usuario (`crontab -l`)
- Próximas ejecuciones programadas
- Historial de ejecuciones recientes (si disponible en logs)

### Panel de Logs del Sistema
- Tail de `/var/log/syslog` o `journalctl`
- Filtros por nivel (error, warning, info)
- Búsqueda en logs
- Logs de auth (`/var/log/auth.log`)

### Panel de Hardware Info
- Información de CPU (modelo completo, cache, flags)
- Información de memoria (tipo, velocidad, slots)
- Información de discos (modelo, serial, SMART status)
- Información de red (driver, MAC, velocidad del link)
- PCI devices (`lspci`)
- USB devices (`lsusb`)

### Panel de Kernel Info
- Versión del kernel
- Módulos cargados (`lsmod`)
- Parámetros del kernel (`sysctl -a` selectos)
- Información de boot

## Fase 3: Funcionalidades Avanzadas

### Historial y Gráficos
- Guardar métricas en SQLite local
- Gráficos de CPU/memoria/red de las últimas N horas
- Configurar retención de datos

### Alertas y Notificaciones
- Definir umbrales (CPU > 90%, disco > 85%, etc.)
- Enviar notificaciones por:
  - Webhook
  - Email (SMTP config)
  - Telegram
  - Desktop notification

### Export de Datos
- Exportar snapshot actual a JSON
- Exportar histórico a CSV
- API para integración con otros sistemas

### Multi-server Dashboard
- Agregar múltiples servidores al config
- Vista consolidada de todos los servidores
- Comparación de métricas entre servidores

### Terminal Web
- Terminal integrado vía WebSocket
- Requiere autenticación
- Opcional: solo para comandos específicos permitidos

### Audit Log
- Registrar todas las acciones realizadas
- Quién hizo qué y cuándo
- Exportable para compliance

## Fase 4: UX Improvements

### Dashboard Customizable
- Drag & drop para reordenar paneles
- Ocultar/mostrar paneles
- Guardar layouts personalizados

### Shortcuts de Teclado
- `p` para pausar todo
- `t` para toggle theme
- `c` para compact mode
- `/` para focus en búsqueda de procesos
- `?` para mostrar ayuda

### Responsive Design Mejorado
- Vista mobile optimizada
- PWA support (installable)

### Internacionalización
- Soporte para múltiples idiomas
- Detección automática del locale del browser

## Ideas Adicionales

### Process Tree View
- Visualizar procesos como árbol jerárquico
- Expandir/colapsar ramas
- Ver toda la descendencia de un proceso

### Resource Limits
- Mostrar límites de recursos por proceso (ulimit)
- cgroups info si aplica

### Network Analysis
- Mostrar conexiones por proceso de forma gráfica
- Detectar procesos con muchas conexiones
- Bandwidth por proceso

### Security Audit
- Procesos corriendo como root
- Puertos abiertos en 0.0.0.0
- Procesos sin capabilities esperadas
- Archivos setuid/setgid
