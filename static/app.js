const { createApp, ref, computed, onMounted, onUnmounted, watch } = Vue;

const app = createApp({
    setup() {
        // State
        const config = ref({});
        const theme = ref('dark');
        const compact = ref(true);
        const authenticated = ref(false);
        const readWrite = ref(false);
        const username = ref('');
        const showLogin = ref(false);
        const requiresLogin = ref(false);
        const isPublic = ref(true);
        const isAdmin = ref(false);
        const hasReadWriteAuth = ref(false);
        const loginForm = ref({ username: '', password: '' });
        const loginError = ref('');

        // Data
        const cpu = ref({});
        const memory = ref({});
        const disk = ref({});
        const network = ref({});
        const gpu = ref({});
        const processes = ref({ processes: [], totalCount: 0 });
        const sockets = ref({ tcp: [], udp: [], unix: [] });
        const firewall = ref({});
        const docker = ref({ containers: [], available: false });

        // Global search
        const globalSearch = ref('');
        const activeTab = ref('overview'); // overview, processes, sockets, docker, firewall

        // UI state
        const paused = ref({
            cpu: false,
            memory: false,
            disk: false,
            network: false,
            gpu: false,
            processes: false,
            sockets: false,
            firewall: false,
            docker: false
        });
        const pausedAll = ref(false);
        const maximizedPanel = ref(null); // null or panel name: 'cpu', 'memory', 'disk', 'network', 'gpu', 'processes', 'sockets', 'firewall', 'docker'

        const toggleMaximize = (panel) => {
            maximizedPanel.value = maximizedPanel.value === panel ? null : panel;
        };

        const processFilter = ref('');
        const dockerFilter = ref('');
        const socketFilter = ref('');
        const sortKey = ref('cpuPercent');
        const sortAsc = ref(false);
        const socketTab = ref('tcp');
        const selectedProcess = ref(null);
        const newPriority = ref(0);
        const selectedIP = ref(null);
        const ipLoading = ref(false);
        const selectedUser = ref(null);
        const userLoading = ref(false);

        // Toast notifications
        const toasts = ref([]);
        let toastId = 0;

        // Group modal
        const selectedGroup = ref(null);
        const groupMembers = ref([]);
        const groupLoading = ref(false);

        // Docker modal
        const selectedContainer = ref(null);
        const containerLoading = ref(false);

        // Docker extended info
        const containerLogs = ref('');
        const containerLogsLoading = ref(false);
        const containerLogsTail = ref(100);
        const containerTop = ref([]);
        const containerTopLoading = ref(false);
        const containerInspect = ref('');
        const showInspectModal = ref(false);

        // Show all items toggles (for "... and X more" links)
        const showAllFds = ref(false);
        const showAllEnv = ref(false);

        // Service PID (to prevent self-kill)
        const servicePid = ref(null);

        // Alert system
        const defaultAlerts = {
            cpu: { enabled: true, threshold: 90 },
            ram: { enabled: true, threshold: 90 },
            disk: { enabled: true, threshold: 90 },
            temp: { enabled: true, threshold: 85 },
            // Extended (off by default)
            swap: { enabled: false, threshold: 80 },
            gpuUsage: { enabled: false, threshold: 95 },
            gpuTemp: { enabled: false, threshold: 85 },
            gpuVram: { enabled: false, threshold: 90 }
        };

        const alerts = ref(loadAlerts());
        const firedAlerts = new Set(); // Track fired alerts this session
        const activePopover = ref(null); // Which alert popover is open

        function loadAlerts() {
            const saved = localStorage.getItem('alerts');
            if (saved) {
                try {
                    return { ...defaultAlerts, ...JSON.parse(saved) };
                } catch (e) {
                    return { ...defaultAlerts };
                }
            }
            return { ...defaultAlerts };
        }

        function saveAlerts() {
            localStorage.setItem('alerts', JSON.stringify(alerts.value));
        }

        let eventSource = null;
        let reconnectAttempts = 0;
        let reconnectTimeout = null;
        const maxReconnectDelay = 30000; // Max 30 seconds between retries
        const connected = ref(true);

        // Computed
        const effectiveFilter = computed(() => {
            return globalSearch.value || processFilter.value;
        });

        const filteredProcesses = computed(() => {
            let procs = processes.value.processes || [];

            // Filter (use global search or local filter)
            const filter = effectiveFilter.value?.toLowerCase();
            if (filter) {
                procs = procs.filter(p =>
                    p.name?.toLowerCase().includes(filter) ||
                    p.command?.toLowerCase().includes(filter) ||
                    p.user?.toLowerCase().includes(filter) ||
                    String(p.pid).includes(filter)
                );
            }

            // Sort
            procs = [...procs].sort((a, b) => {
                let aVal = a[sortKey.value];
                let bVal = b[sortKey.value];

                if (typeof aVal === 'string') {
                    aVal = aVal.toLowerCase();
                    bVal = bVal?.toLowerCase() || '';
                }

                if (aVal < bVal) return sortAsc.value ? -1 : 1;
                if (aVal > bVal) return sortAsc.value ? 1 : -1;
                return 0;
            });

            return procs.slice(0, 100); // Limit display
        });

        const filteredSockets = computed(() => {
            const filter = (globalSearch.value || socketFilter.value)?.toLowerCase();
            if (!filter) return currentSockets.value;

            return currentSockets.value.filter(s =>
                s.localAddr?.toLowerCase().includes(filter) ||
                s.remoteAddr?.toLowerCase().includes(filter) ||
                String(s.localPort).includes(filter) ||
                String(s.remotePort).includes(filter) ||
                s.processName?.toLowerCase().includes(filter) ||
                String(s.pid).includes(filter) ||
                s.state?.toLowerCase().includes(filter)
            );
        });

        const filteredContainers = computed(() => {
            const filter = (globalSearch.value || dockerFilter.value)?.toLowerCase();
            if (!filter) return docker.value.containers || [];

            return (docker.value.containers || []).filter(c =>
                c.name?.toLowerCase().includes(filter) ||
                c.image?.toLowerCase().includes(filter) ||
                c.id?.toLowerCase().includes(filter) ||
                c.status?.toLowerCase().includes(filter)
            );
        });

        const filteredInterfaces = computed(() => {
            const filter = globalSearch.value?.toLowerCase();
            if (!filter) return network.value.interfaces || [];

            return (network.value.interfaces || []).filter(iface =>
                iface.name?.toLowerCase().includes(filter) ||
                iface.ipAddresses?.some(ip => ip.toLowerCase().includes(filter)) ||
                iface.mac?.toLowerCase().includes(filter)
            );
        });

        const hasSearchResults = computed(() => {
            if (!globalSearch.value) return true;
            return filteredProcesses.value.length > 0 ||
                   filteredSockets.value.length > 0 ||
                   filteredContainers.value.length > 0;
        });

        const currentSockets = computed(() => {
            if (socketTab.value === 'tcp') return sockets.value.tcp || [];
            if (socketTab.value === 'udp') return sockets.value.udp || [];
            return [];
        });

        // Methods
        const formatBytes = (bytes) => {
            if (!bytes || bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
        };

        const formatBytesSpeed = (bytes) => {
            return formatBytes(bytes) + '/s';
        };

        const getUsageColor = (percent) => {
            if (percent >= 90) return '#ef4444';
            if (percent >= 70) return '#f59e0b';
            if (percent >= 50) return '#eab308';
            return '#22c55e';
        };

        const getCpuClass = (percent) => {
            if (percent >= 80) return 'high';
            if (percent >= 50) return 'medium';
            return '';
        };

        const getMemClass = (percent) => {
            if (percent >= 80) return 'high';
            if (percent >= 50) return 'medium';
            return '';
        };

        // Toast functions
        const showToast = (message, type = 'info', duration = 4000) => {
            const id = ++toastId;
            toasts.value.push({ id, message, type });
            if (duration > 0) {
                setTimeout(() => removeToast(id), duration);
            }
            return id;
        };

        const removeToast = (id) => {
            const index = toasts.value.findIndex(t => t.id === id);
            if (index !== -1) {
                toasts.value.splice(index, 1);
            }
        };

        // Check if PID is the service itself
        const isServiceProcess = (pid) => {
            return servicePid.value && pid === servicePid.value;
        };

        const sortBy = (key) => {
            if (sortKey.value === key) {
                sortAsc.value = !sortAsc.value;
            } else {
                sortKey.value = key;
                sortAsc.value = false;
            }
        };

        const toggleTheme = () => {
            theme.value = theme.value === 'dark' ? 'light' : 'dark';
            document.body.setAttribute('data-theme', theme.value);
            localStorage.setItem('theme', theme.value);
        };

        const toggleCompact = () => {
            compact.value = !compact.value;
            localStorage.setItem('compact', compact.value);
        };

        const togglePauseAll = () => {
            pausedAll.value = !pausedAll.value;
            Object.keys(paused.value).forEach(key => {
                paused.value[key] = pausedAll.value;
            });

            // Disconnect/reconnect SSE based on pause state
            if (pausedAll.value) {
                // Disconnect SSE to save bandwidth
                if (reconnectTimeout) {
                    clearTimeout(reconnectTimeout);
                    reconnectTimeout = null;
                }
                if (eventSource) {
                    eventSource.close();
                    eventSource = null;
                }
                connected.value = false;
            } else {
                // Reconnect SSE
                connectSSE();
            }
        };

        const loadConfig = async () => {
            try {
                const res = await fetch('/api/config');
                config.value = await res.json();

                // Apply UI settings
                if (config.value.ui) {
                    document.title = config.value.ui.title || 'Syspeek';
                    if (config.value.ui.theme) {
                        theme.value = config.value.ui.theme;
                    }
                    if (config.value.ui.compactMode) {
                        compact.value = config.value.ui.compactMode;
                    }
                }
            } catch (e) {
                console.error('Failed to load config:', e);
            }
        };

        const checkAuth = async () => {
            try {
                const res = await fetch('/api/auth/status');
                const data = await res.json();
                authenticated.value = data.authenticated;
                readWrite.value = data.readWrite;
                username.value = data.username || '';
                requiresLogin.value = data.requiresLogin;
                isPublic.value = data.isPublic;
                isAdmin.value = data.isAdmin;
                hasReadWriteAuth.value = data.hasReadWriteAuth;

                // Show login modal if login is required and not authenticated
                if (requiresLogin.value && !authenticated.value) {
                    showLogin.value = true;
                }
            } catch (e) {
                console.error('Failed to check auth:', e);
            }
        };

        const login = async () => {
            loginError.value = '';
            try {
                const res = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(loginForm.value)
                });
                const data = await res.json();
                if (data.success) {
                    authenticated.value = true;
                    readWrite.value = data.readWrite;
                    username.value = loginForm.value.username;
                    showLogin.value = false;
                    loginForm.value = { username: '', password: '' };
                } else {
                    loginError.value = data.message || 'Login failed';
                }
            } catch (e) {
                loginError.value = 'Connection error';
            }
        };

        const logout = async () => {
            try {
                await fetch('/api/auth/logout', { method: 'POST' });
                authenticated.value = false;
                readWrite.value = false;
                username.value = '';
                // Show login again if required
                if (requiresLogin.value) {
                    showLogin.value = true;
                }
            } catch (e) {
                console.error('Logout failed:', e);
            }
        };

        const showProcessDetail = async (pid) => {
            try {
                const res = await fetch(`/api/process/${pid}`);
                if (res.ok) {
                    selectedProcess.value = await res.json();
                    newPriority.value = selectedProcess.value.nice || 0;
                }
            } catch (e) {
                console.error('Failed to get process detail:', e);
            }
        };

        const killProcess = async (pid, signal) => {
            if (!confirm(`Send signal ${signal} to process ${pid}?`)) return;

            try {
                const res = await fetch(`/api/process/${pid}/kill`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ signal })
                });
                const data = await res.json();
                if (data.success) {
                    selectedProcess.value = null;
                } else {
                    alert(data.message || 'Failed to kill process');
                }
            } catch (e) {
                alert('Error: ' + e.message);
            }
        };

        const reniceProcess = async (pid) => {
            try {
                const res = await fetch(`/api/process/${pid}/renice`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ priority: parseInt(newPriority.value) })
                });
                const data = await res.json();
                if (data.success) {
                    // Refresh process detail
                    showProcessDetail(pid);
                } else {
                    alert(data.message || 'Failed to renice process');
                }
            } catch (e) {
                alert('Error: ' + e.message);
            }
        };

        const showIPInfo = async (ip) => {
            if (!ip || ip === '*' || ip === '0.0.0.0' || ip === '::') {
                return; // Skip invalid or wildcard IPs
            }

            ipLoading.value = true;
            selectedIP.value = { ip }; // Show modal immediately with IP

            try {
                const res = await fetch(`/api/ip/${encodeURIComponent(ip)}`);
                if (res.ok) {
                    selectedIP.value = await res.json();
                } else {
                    selectedIP.value = { ip, error: 'Failed to load IP info' };
                }
            } catch (e) {
                console.error('Failed to get IP info:', e);
                selectedIP.value = { ip, error: e.message };
            } finally {
                ipLoading.value = false;
            }
        };

        const showUserInfo = async (username) => {
            if (!username) return;

            userLoading.value = true;
            selectedUser.value = { username }; // Show modal immediately

            try {
                const res = await fetch(`/api/user/${encodeURIComponent(username)}`);
                if (res.ok) {
                    selectedUser.value = await res.json();
                } else {
                    selectedUser.value = { username, error: 'Failed to load user info' };
                }
            } catch (e) {
                console.error('Failed to get user info:', e);
                selectedUser.value = { username, error: e.message };
            } finally {
                userLoading.value = false;
            }
        };

        // Group info
        const showGroupInfo = async (groupname) => {
            if (!groupname) return;

            groupLoading.value = true;
            selectedGroup.value = { name: groupname };
            groupMembers.value = [];

            try {
                const res = await fetch(`/api/group/${encodeURIComponent(groupname)}`);
                if (res.ok) {
                    const data = await res.json();
                    selectedGroup.value = data;
                    groupMembers.value = data.members || [];
                } else {
                    selectedGroup.value = { name: groupname, error: 'Failed to load group info' };
                }
            } catch (e) {
                console.error('Failed to get group info:', e);
                selectedGroup.value = { name: groupname, error: e.message };
            } finally {
                groupLoading.value = false;
            }
        };

        const removeUserFromGroup = async (groupname, username) => {
            if (!confirm(`Remove ${username} from group ${groupname}?`)) return;

            try {
                const res = await fetch(`/api/group/${encodeURIComponent(groupname)}/remove`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username })
                });
                const data = await res.json();
                if (data.success) {
                    showToast(`Removed ${username} from ${groupname}`, 'success');
                    // Refresh group members
                    showGroupInfo(groupname);
                } else {
                    showToast(data.message || 'Failed to remove user from group', 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // User modification
        const modifyUser = async (username, field, value) => {
            try {
                const res = await fetch(`/api/user/${encodeURIComponent(username)}/modify`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ [field]: value })
                });
                const data = await res.json();
                if (data.success) {
                    showToast(`Updated ${field} for ${username}`, 'success');
                    // Refresh user info
                    showUserInfo(username);
                } else {
                    showToast(data.message || `Failed to update ${field}`, 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // Quick kill from process list
        const quickKillProcess = async (pid, name) => {
            if (isServiceProcess(pid)) {
                showToast('Cannot kill the service process itself', 'error');
                return;
            }
            if (!confirm(`Kill process ${name} (${pid})?`)) return;

            try {
                const res = await fetch(`/api/process/${pid}/kill`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ signal: 15 }) // SIGTERM
                });
                const data = await res.json();
                if (data.success) {
                    showToast(`Sent SIGTERM to ${name} (${pid})`, 'success');
                } else {
                    showToast(data.message || 'Failed to kill process', 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // Pause/Continue process (SIGSTOP/SIGCONT)
        const toggleProcessPause = async (pid, name, isPaused) => {
            if (isServiceProcess(pid)) {
                showToast('Cannot pause the service process itself', 'error');
                return;
            }

            const signal = isPaused ? 18 : 19; // SIGCONT : SIGSTOP
            const action = isPaused ? 'resume' : 'pause';

            try {
                const res = await fetch(`/api/process/${pid}/kill`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ signal })
                });
                const data = await res.json();
                if (data.success) {
                    showToast(`Process ${name} (${pid}) ${action}d`, 'success');
                    if (selectedProcess.value && selectedProcess.value.pid === pid) {
                        showProcessDetail(pid);
                    }
                } else {
                    showToast(data.message || `Failed to ${action} process`, 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // Docker functions
        const showContainerDetail = async (containerId) => {
            containerLoading.value = true;
            selectedContainer.value = { id: containerId };
            // Reset extended state
            resetContainerExtended();

            try {
                const res = await fetch(`/api/docker/${encodeURIComponent(containerId)}`);
                if (res.ok) {
                    selectedContainer.value = await res.json();
                    // Auto-load processes and logs if container is running
                    if (selectedContainer.value.state === 'running') {
                        fetchContainerTop(containerId);
                    }
                    fetchContainerLogs(containerId);
                } else {
                    selectedContainer.value = { id: containerId, error: 'Failed to load container info' };
                }
            } catch (e) {
                console.error('Failed to get container info:', e);
                selectedContainer.value = { id: containerId, error: e.message };
            } finally {
                containerLoading.value = false;
            }
        };

        const dockerAction = async (containerId, action) => {
            const actionNames = { stop: 'Stop', start: 'Start', restart: 'Restart', kill: 'Kill', pause: 'Pause', unpause: 'Unpause' };
            if (!confirm(`${actionNames[action]} container ${containerId.substring(0, 12)}?`)) return;

            try {
                const res = await fetch(`/api/docker/${encodeURIComponent(containerId)}/${action}`, {
                    method: 'POST'
                });
                const data = await res.json();
                if (data.success) {
                    showToast(`Container ${action}ed successfully`, 'success');
                    if (selectedContainer.value) {
                        showContainerDetail(containerId);
                    }
                } else {
                    showToast(data.message || `Failed to ${action} container`, 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // Fetch container logs
        const fetchContainerLogs = async (containerId, tail = null) => {
            if (tail) {
                containerLogsTail.value = tail;
            }
            containerLogsLoading.value = true;

            try {
                const res = await fetch(`/api/docker/${encodeURIComponent(containerId)}/logs?tail=${containerLogsTail.value}`);
                if (res.ok) {
                    const data = await res.json();
                    containerLogs.value = data.logs || '';
                    // Scroll logs to bottom after DOM update
                    setTimeout(() => {
                        const logsEl = document.querySelector('.logs-content');
                        if (logsEl) {
                            logsEl.scrollTop = logsEl.scrollHeight;
                        }
                    }, 50);
                } else {
                    showToast('Failed to fetch logs', 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            } finally {
                containerLogsLoading.value = false;
            }
        };

        // Load more logs (double the current amount)
        const loadMoreLogs = async (containerId) => {
            const newTail = containerLogsTail.value * 2;
            await fetchContainerLogs(containerId, newTail);
        };

        // Fetch container top (processes)
        const fetchContainerTop = async (containerId) => {
            containerTopLoading.value = true;

            try {
                const res = await fetch(`/api/docker/${encodeURIComponent(containerId)}/top`);
                if (res.ok) {
                    const data = await res.json();
                    containerTop.value = data.processes || [];
                } else {
                    showToast('Failed to fetch processes', 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            } finally {
                containerTopLoading.value = false;
            }
        };

        // Fetch raw inspect JSON
        const fetchContainerInspect = async (containerId) => {
            try {
                const res = await fetch(`/api/docker/${encodeURIComponent(containerId)}/inspect`);
                if (res.ok) {
                    const data = await res.json();
                    containerInspect.value = data.inspect || '';
                    showInspectModal.value = true;
                } else {
                    showToast('Failed to fetch inspect data', 'error');
                }
            } catch (e) {
                showToast('Error: ' + e.message, 'error');
            }
        };

        // Reset container extended state
        const resetContainerExtended = () => {
            containerLogs.value = '';
            containerLogsTail.value = 100;
            containerTop.value = [];
            containerInspect.value = '';
            showInspectModal.value = false;
        };

        const clearGlobalSearch = () => {
            globalSearch.value = '';
        };

        // Docker label helpers
        const hasComposeLabels = (labels) => {
            if (!labels) return false;
            return Object.keys(labels).some(k => k.startsWith('com.docker.compose.'));
        };

        const filteredLabels = (labels) => {
            if (!labels) return {};
            // Filter out compose labels (shown separately) and show other labels
            const result = {};
            for (const [key, value] of Object.entries(labels)) {
                if (!key.startsWith('com.docker.compose.')) {
                    result[key] = value;
                }
            }
            return result;
        };

        // Alert system functions
        const requestNotificationPermission = async () => {
            if (!('Notification' in window)) {
                console.warn('Notifications not supported');
                return false;
            }
            if (Notification.permission === 'granted') {
                return true;
            }
            if (Notification.permission !== 'denied') {
                const permission = await Notification.requestPermission();
                return permission === 'granted';
            }
            return false;
        };

        const sendNotification = (title, body, tag) => {
            if (Notification.permission === 'granted') {
                new Notification(title, {
                    body,
                    tag, // Prevents duplicate notifications
                    icon: '/static/icon.png',
                    requireInteraction: false
                });
            }
        };

        const checkAlert = async (key, value, threshold, label, unit = '%') => {
            if (!alerts.value[key]?.enabled) return;
            if (firedAlerts.has(key)) return;
            if (value > threshold) {
                firedAlerts.add(key);
                const hasPermission = await requestNotificationPermission();
                if (hasPermission) {
                    sendNotification(
                        `${label} Alert`,
                        `${label} is at ${value.toFixed(1)}${unit} (threshold: ${threshold}${unit})`,
                        `alert-${key}`
                    );
                }
                showToast(`${label}: ${value.toFixed(1)}${unit} exceeds ${threshold}${unit}`, 'error', 6000);
            }
        };

        const checkAlerts = (type, data) => {
            if (!data) return;

            switch (type) {
                case 'cpu':
                    if (data.usagePercent !== undefined) {
                        checkAlert('cpu', data.usagePercent, alerts.value.cpu.threshold, 'CPU');
                    }
                    if (data.packageTemp !== undefined) {
                        checkAlert('temp', data.packageTemp, alerts.value.temp.threshold, 'Temperature', '°C');
                    }
                    break;
                case 'memory':
                    if (data.usedPercent !== undefined) {
                        checkAlert('ram', data.usedPercent, alerts.value.ram.threshold, 'RAM');
                    }
                    if (data.swapPercent !== undefined && alerts.value.swap.enabled) {
                        checkAlert('swap', data.swapPercent, alerts.value.swap.threshold, 'Swap');
                    }
                    break;
                case 'disk':
                    if (data.partitions) {
                        for (const part of data.partitions) {
                            if (part.usedPercent > alerts.value.disk.threshold && !firedAlerts.has('disk')) {
                                checkAlert('disk', part.usedPercent, alerts.value.disk.threshold, `Disk ${part.mountPoint}`);
                                break; // Only alert once for disk
                            }
                        }
                    }
                    break;
                case 'gpu':
                    if (data.available) {
                        if (data.usagePercent !== undefined && alerts.value.gpuUsage.enabled) {
                            checkAlert('gpuUsage', data.usagePercent, alerts.value.gpuUsage.threshold, 'GPU Usage');
                        }
                        if (data.temperature !== undefined && alerts.value.gpuTemp.enabled) {
                            checkAlert('gpuTemp', data.temperature, alerts.value.gpuTemp.threshold, 'GPU Temp', '°C');
                        }
                        if (data.memoryTotal > 0 && alerts.value.gpuVram.enabled) {
                            const vramPercent = (data.memoryUsed / data.memoryTotal) * 100;
                            checkAlert('gpuVram', vramPercent, alerts.value.gpuVram.threshold, 'GPU VRAM');
                        }
                    }
                    break;
            }
        };

        const toggleAlert = (key) => {
            alerts.value[key].enabled = !alerts.value[key].enabled;
            saveAlerts();
        };

        const updateAlertThreshold = (key, value) => {
            const num = parseInt(value);
            if (!isNaN(num) && num >= 0 && num <= 100) {
                alerts.value[key].threshold = num;
                saveAlerts();
            }
        };

        const togglePopover = (key) => {
            activePopover.value = activePopover.value === key ? null : key;
        };

        const getAlertState = (key) => {
            if (firedAlerts.has(key)) return 'fired';
            if (alerts.value[key]?.enabled) return 'active';
            return 'disabled';
        };

        const connectSSE = () => {
            // Close existing connection if any
            if (eventSource) {
                eventSource.close();
                eventSource = null;
            }

            // Clear any pending reconnect
            if (reconnectTimeout) {
                clearTimeout(reconnectTimeout);
                reconnectTimeout = null;
            }

            eventSource = new EventSource('/api/stream');

            eventSource.onopen = () => {
                console.log('SSE connected');
                connected.value = true;
                reconnectAttempts = 0;
            };

            eventSource.addEventListener('cpu', (e) => {
                const data = JSON.parse(e.data);
                if (!paused.value.cpu && !pausedAll.value) {
                    cpu.value = data.data;
                }
                checkAlerts('cpu', data.data);
            });

            eventSource.addEventListener('memory', (e) => {
                const data = JSON.parse(e.data);
                if (!paused.value.memory && !pausedAll.value) {
                    memory.value = data.data;
                }
                checkAlerts('memory', data.data);
            });

            eventSource.addEventListener('disk', (e) => {
                const data = JSON.parse(e.data);
                if (!paused.value.disk && !pausedAll.value) {
                    disk.value = data.data;
                }
                checkAlerts('disk', data.data);
            });

            eventSource.addEventListener('network', (e) => {
                if (!paused.value.network && !pausedAll.value) {
                    const data = JSON.parse(e.data);
                    network.value = data.data;
                }
            });

            eventSource.addEventListener('gpu', (e) => {
                const data = JSON.parse(e.data);
                if (!paused.value.gpu && !pausedAll.value) {
                    gpu.value = data.data;
                }
                checkAlerts('gpu', data.data);
            });

            eventSource.addEventListener('processes', (e) => {
                if (!paused.value.processes && !pausedAll.value) {
                    const data = JSON.parse(e.data);
                    processes.value = data.data;
                }
            });

            eventSource.addEventListener('sockets', (e) => {
                if (!paused.value.sockets && !pausedAll.value) {
                    const data = JSON.parse(e.data);
                    sockets.value = data.data;
                }
            });

            eventSource.addEventListener('firewall', (e) => {
                if (!paused.value.firewall && !pausedAll.value) {
                    const data = JSON.parse(e.data);
                    firewall.value = data.data;
                }
            });

            eventSource.addEventListener('docker', (e) => {
                if (!paused.value.docker && !pausedAll.value) {
                    const data = JSON.parse(e.data);
                    docker.value = data.data;
                }
            });

            eventSource.onerror = () => {
                // Prevent multiple reconnection attempts
                if (reconnectTimeout) {
                    return; // Already scheduled a reconnect
                }

                connected.value = false;
                if (eventSource) {
                    eventSource.close();
                    eventSource = null;
                }

                // Exponential backoff: 2s, 4s, 8s, 16s, 30s max
                reconnectAttempts++;
                const delay = Math.min(2000 * Math.pow(2, reconnectAttempts - 1), maxReconnectDelay);
                console.error(`SSE connection lost. Reconnecting in ${delay/1000}s (attempt ${reconnectAttempts})`);

                reconnectTimeout = setTimeout(connectSSE, delay);
            };
        };

        // Lifecycle
        // Close popover when clicking outside
        const handleDocumentClick = () => {
            if (activePopover.value) {
                activePopover.value = null;
            }
        };

        // Handle page close - notify server to shutdown (desktop mode only)
        const handleBeforeUnload = () => {
            // Use sendBeacon for reliable delivery on page close
            navigator.sendBeacon('/api/close', '');
        };

        onMounted(async () => {
            // Listen for clicks to close popovers
            document.addEventListener('click', handleDocumentClick);

            // Listen for page close to shutdown server (desktop mode)
            window.addEventListener('beforeunload', handleBeforeUnload);

            // Load saved preferences
            const savedTheme = localStorage.getItem('theme');
            if (savedTheme) {
                theme.value = savedTheme;
            }
            document.body.setAttribute('data-theme', theme.value);

            const savedCompact = localStorage.getItem('compact');
            if (savedCompact === 'true') {
                compact.value = true;
            }

            await loadConfig();
            await checkAuth();

            // Fetch service PID to prevent self-kill
            try {
                const res = await fetch('/api/pid');
                if (res.ok) {
                    const data = await res.json();
                    servicePid.value = data.pid;
                }
            } catch (e) {
                console.error('Failed to get service PID:', e);
            }

            connectSSE();
        });

        onUnmounted(() => {
            document.removeEventListener('click', handleDocumentClick);
            window.removeEventListener('beforeunload', handleBeforeUnload);
            if (eventSource) {
                eventSource.close();
                eventSource = null;
            }
            if (reconnectTimeout) {
                clearTimeout(reconnectTimeout);
                reconnectTimeout = null;
            }
        });

        return {
            // State
            config,
            theme,
            compact,
            connected,
            authenticated,
            readWrite,
            username,
            showLogin,
            requiresLogin,
            isPublic,
            isAdmin,
            hasReadWriteAuth,
            loginForm,
            loginError,

            // Data
            cpu,
            memory,
            disk,
            network,
            gpu,
            processes,
            sockets,
            firewall,
            docker,

            // UI state
            paused,
            pausedAll,
            maximizedPanel,
            toggleMaximize,
            processFilter,
            dockerFilter,
            socketFilter,
            sortKey,
            sortAsc,
            socketTab,
            selectedProcess,
            newPriority,
            selectedIP,
            ipLoading,
            selectedUser,
            userLoading,
            globalSearch,
            activeTab,

            // Toast
            toasts,

            // Group modal
            selectedGroup,
            groupMembers,
            groupLoading,

            // Docker modal
            selectedContainer,
            containerLoading,
            containerLogs,
            containerLogsLoading,
            containerLogsTail,
            containerTop,
            containerTopLoading,
            containerInspect,
            showInspectModal,

            // Show all items toggles
            showAllFds,
            showAllEnv,

            // Service PID
            servicePid,

            // Computed
            filteredProcesses,
            filteredSockets,
            filteredContainers,
            filteredInterfaces,
            hasSearchResults,
            currentSockets,

            // Methods
            formatBytes,
            formatBytesSpeed,
            getUsageColor,
            getCpuClass,
            getMemClass,
            sortBy,
            toggleTheme,
            toggleCompact,
            togglePauseAll,
            login,
            logout,
            showProcessDetail,
            killProcess,
            reniceProcess,
            showIPInfo,
            showUserInfo,

            // New methods
            showToast,
            removeToast,
            isServiceProcess,
            showGroupInfo,
            removeUserFromGroup,
            modifyUser,
            quickKillProcess,
            toggleProcessPause,

            // Alert system
            alerts,
            activePopover,
            toggleAlert,
            updateAlertThreshold,
            togglePopover,
            getAlertState,

            // Docker
            showContainerDetail,
            dockerAction,
            clearGlobalSearch,
            hasComposeLabels,
            filteredLabels,
            fetchContainerLogs,
            loadMoreLogs,
            fetchContainerTop,
            fetchContainerInspect,
            resetContainerExtended
        };
    }
});

app.mount('#app');
