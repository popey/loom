// Configuration
const API_BASE = '/api/v1';
const REFRESH_INTERVAL = 5000; // 5 seconds

const AUTH_TOKEN_KEY = 'loom.authToken';
let authToken = localStorage.getItem(AUTH_TOKEN_KEY) || '';
let authCheckInFlight = null;
let loginInFlight = null;

// Feature flag: Set to false to disable authentication in the UI
// Should match server-side config.yaml security.enable_auth setting
const AUTH_ENABLED = false;

// Backend connectivity tracking — suppress toast floods when backend is down
let backendDown = false;
let backendDownSince = null;
let backendBannerEl = null;

// Utility function to format dates consistently
function formatDate(dateString) {
    if (!dateString) return 'Never';
    try {
        const date = new Date(dateString);
        if (isNaN(date.getTime())) return dateString;
        return date.toLocaleString();
    } catch (e) {
        return dateString;
    }
}

function setBackendDown(isDown) {
    if (isDown && !backendDown) {
        backendDown = true;
        backendDownSince = Date.now();
        showBackendBanner(true);
    } else if (!isDown && backendDown) {
        backendDown = false;
        backendDownSince = null;
        showBackendBanner(false);
        loadAll();
    }
}

function showBackendBanner(show) {
    if (!backendBannerEl) {
        backendBannerEl = document.createElement('div');
        backendBannerEl.id = 'backend-down-banner';
        backendBannerEl.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:10000;background:#d32f2f;color:#fff;text-align:center;padding:0.75rem;font-weight:600;font-size:0.95rem;display:none;';
        document.body.prepend(backendBannerEl);
    }
    if (show) {
        backendBannerEl.textContent = 'Backend unavailable — reconnecting...';
        backendBannerEl.style.display = 'block';
    } else {
        backendBannerEl.style.display = 'none';
    }
}

// Health check pill — polls /health every 60s
const HEALTH_CHECK_INTERVAL = 60000;
let lastHealthData = null;

async function checkBackendHealth() {
    const pill = document.getElementById('health-pill');
    if (!pill) return;
    try {
        const resp = await fetch('/health', { signal: AbortSignal.timeout(5000) });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();
        lastHealthData = data;
        const ok = data.status === 'healthy';
        pill.style.background = ok ? '#2e7d32' : '#d32f2f';
        pill.textContent = ok ? 'Healthy' : data.status || 'Unhealthy';
        pill.title = `Backend: ${data.status} | Uptime: ${Math.floor((data.uptime_seconds || 0) / 60)}m | v${data.version || '?'}`;
        if (backendDown) setBackendDown(false);
    } catch {
        pill.style.background = '#d32f2f';
        pill.textContent = 'Offline';
        pill.title = 'Backend: unreachable';
        setBackendDown(true);
    }
}

function startHealthCheck() {
    checkBackendHealth();
    setInterval(checkBackendHealth, HEALTH_CHECK_INTERVAL);
}

// State
let state = {
    beads: [],
    ceoBeads: null,
    agents: [],
    projects: [],
    personas: [],
    decisions: [],
    systemStatus: null,
    users: [],
    apiKeys: [],
    activeMeetings: [],
    statusBoardFeed: [],
    orgHealth: {},
    reviewSummary: {},
    escalationQueue: []
};
window.state = state;

let uiState = {
    view: {
        active: 'project-viewer'
    },
    bead: {
        search: '',
        sort: 'priority',
        priority: 'all',
        type: 'all',
        assigned: '',
        tag: '',
        project: 'all'
    },
    agent: {
        search: ''
    },
    project: {
        selectedId: ''
    }
};

let busy = new Set();

// Exposed on window so hotreload.js can check if a modal is open
window.modalState = {
    activeId: null,
    lastFocused: null
};
const modalState = window.modalState;

// Helper to get authentication headers for API calls
function getAuthHeaders() {
    const headers = { 'Content-Type': 'application/json' };
    if (AUTH_ENABLED && authToken) {
        headers['Authorization'] = `Bearer ${authToken}`;
    }
    return headers;
}

let eventStreamConnected = false;
let reloadTimers = {};

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    console.log('[Loom] DOMContentLoaded - Initializing...');
    console.log('[Loom] AUTH_ENABLED =', AUTH_ENABLED);
    try {
        initUI();
        initViewTabs();
        // Initialize diagrams UI after Cytoscape loads
        if (typeof cytoscape !== 'undefined' && typeof initDiagramsUI === 'function') {
            initDiagramsUI();
        } else {
            console.warn('[Loom] Cytoscape.js or diagrams.js not loaded');
        }
        loadAll();
        startEventStream();
        startAutoRefresh();
        startHealthCheck();
        console.log('[Loom] Initialization complete');
    } catch (error) {
        console.error('[Loom] Initialization failed:', error);
    }
});

function initUI() {
    const beadSearch = document.getElementById('bead-search');
    const beadSort = document.getElementById('bead-sort');
    const beadPriority = document.getElementById('bead-priority');
    const beadType = document.getElementById('bead-type');
    const beadProject = document.getElementById('bead-project');
    const beadAssigned = document.getElementById('bead-assigned');
    const beadTag = document.getElementById('bead-tag');
    const beadClear = document.getElementById('bead-clear-filters');

    // User management controls
    const createUserBtn = document.getElementById('create-user-btn');
    const cancelUserBtn = document.getElementById('cancel-user-btn');
    const userForm = document.getElementById('user-form');
    const refreshUsersBtn = document.getElementById('refresh-users-btn');
    
    createUserBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-user-form');
        if (form) form.style.display = 'block';
    });
    
    cancelUserBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-user-form');
        if (form) form.style.display = 'none';
        document.getElementById('user-form')?.reset();
    });
    
    userForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await handleCreateUser();
    });
    
    refreshUsersBtn?.addEventListener('click', () => {
        loadUsers();
        loadAPIKeys();
        render();
    });

    // API key controls
    const createAPIKeyBtn = document.getElementById('create-apikey-btn');
    const cancelAPIKeyBtn = document.getElementById('cancel-apikey-btn');
    const apiKeyForm = document.getElementById('apikey-form');
    const closeAPIKeyDisplayBtn = document.getElementById('close-apikey-display-btn');
    const copyAPIKeyBtn = document.getElementById('copy-apikey-btn');
    
    createAPIKeyBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-apikey-form');
        if (form) form.style.display = 'block';
    });
    
    cancelAPIKeyBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-apikey-form');
        if (form) form.style.display = 'none';
        document.getElementById('apikey-form')?.reset();
    });
    
    apiKeyForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await handleCreateAPIKey();
    });
    
    closeAPIKeyDisplayBtn?.addEventListener('click', () => {
        const display = document.getElementById('apikey-display');
        if (display) display.style.display = 'none';
    });
    
    copyAPIKeyBtn?.addEventListener('click', () => {
        const keyValue = document.getElementById('apikey-value')?.textContent;
        if (keyValue) {
            navigator.clipboard.writeText(keyValue).then(() => {
                showToast('API key copied to clipboard', 'success');
            });
        }
    });

    beadSearch?.addEventListener('input', (e) => {
        uiState.bead.search = e.target.value || '';
        render();
    });
    beadSort?.addEventListener('change', (e) => {
        uiState.bead.sort = e.target.value;
        render();
    });
    beadPriority?.addEventListener('change', (e) => {
        uiState.bead.priority = e.target.value;
        render();
    });
    beadType?.addEventListener('change', (e) => {
        uiState.bead.type = e.target.value;
        render();
    });
    beadProject?.addEventListener('change', (e) => {
        uiState.bead.project = e.target.value;
        render();
    });
    beadAssigned?.addEventListener('input', (e) => {
        uiState.bead.assigned = e.target.value || '';
        render();
    });
    beadTag?.addEventListener('input', (e) => {
        uiState.bead.tag = e.target.value || '';
        render();
    });

    beadClear?.addEventListener('click', () => {
        uiState.bead = {
            search: '',
            sort: 'priority',
            priority: 'all',
            type: 'all',
            assigned: '',
            tag: '',
            project: 'all'
        };

        if (beadSearch) beadSearch.value = '';
        if (beadSort) beadSort.value = 'priority';
        if (beadPriority) beadPriority.value = 'all';
        if (beadType) beadType.value = 'all';
        if (beadProject) beadProject.value = 'all';
        if (beadAssigned) beadAssigned.value = '';
        if (beadTag) beadTag.value = '';

        render();
    });

    const agentSearch = document.getElementById('agent-search');
    agentSearch?.addEventListener('input', (e) => {
        uiState.agent.search = e.target.value || '';
        render();
    });

    initKanbanDnD();


    const projectSelect = document.getElementById('project-view-select');
    projectSelect?.addEventListener('change', (e) => {
        uiState.project.selectedId = e.target.value || '';
        render();
    });

    // Legacy REPL (backward compat)
    const replSend = document.getElementById('repl-send');
    replSend?.addEventListener('click', () => {
        sendReplQuery();
    });

    // Unified CEO REPL
    const ceoReplSend = document.getElementById('ceo-repl-send');
    ceoReplSend?.addEventListener('click', () => {
        sendCeoReplQuery();
    });

    // CEO REPL Assign button
    const ceoReplAssign = document.getElementById('ceo-repl-assign');
    ceoReplAssign?.addEventListener('click', () => {
        assignAgentFromCeoRepl();
    });

    // Streaming test controls
    const streamTestSend = document.getElementById('stream-test-send');
    streamTestSend?.addEventListener('click', () => {
        sendStreamingTest();
    });

    const streamTestClear = document.getElementById('stream-test-clear');
    streamTestClear?.addEventListener('click', () => {
        const responseEl = document.getElementById('stream-test-response');
        const statsEl = document.getElementById('stream-test-stats');
        if (responseEl) responseEl.textContent = '';
        if (statsEl) statsEl.textContent = '';
    });
}

function initViewTabs() {
    const tabs = Array.from(document.querySelectorAll('.view-tab'));
    const panels = Array.from(document.querySelectorAll('.view-panel'));

    // Exposed globally so viewProject() and other functions can switch tabs
    window.activateViewTab = activate;

    function activate(id) {
        uiState.view.active = id;
        for (const tab of tabs) {
            const target = tab.getAttribute('data-target');
            const isActive = target === id;
            tab.classList.toggle('active', isActive);
            tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
            tab.tabIndex = isActive ? 0 : -1;
        }
        for (const panel of panels) {
            const isActive = panel.id === id;
            panel.classList.toggle('active', isActive);
            if (isActive) {
                panel.removeAttribute('hidden');
            } else {
                panel.setAttribute('hidden', 'true');
            }
        }
    }

    for (const tab of tabs) {
        tab.addEventListener('click', () => {
            const target = tab.getAttribute('data-target');
            if (target) activate(target);
        });
        tab.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                const target = tab.getAttribute('data-target');
                if (target) activate(target);
            }
        });
    }

    window.addEventListener('hashchange', () => {
        const id = (location.hash || '').replace('#', '');
        if (id && panels.find((p) => p.id === id)) {
            activate(id);
        }
    });

    // Respect hash on load if it matches a panel id
    const hash = (location.hash || '').replace('#', '');
    if (hash && panels.find((p) => p.id === hash)) {
        activate(hash);
    } else {
        activate(uiState.view.active);
    }
}

// Auto-refresh
function startAutoRefresh() {
    // Event bus is preferred; this interval is a fallback.
    setInterval(() => {
        if (!eventStreamConnected && !modalState.activeId) loadAll();
    }, REFRESH_INTERVAL);
}

// Load all data
async function loadAll() {
    console.log('[Loom] loadAll() started');
    try {
        await Promise.all([
            loadBeads().catch(err => { console.error('[Loom] Failed to load beads:', err); state.beads = []; }),
            loadAgents().catch(err => { console.error('[Loom] Failed to load agents:', err); state.agents = []; }),
            loadProjects().catch(err => { console.error('[Loom] Failed to load projects:', err); state.projects = []; }),
            loadPersonas().catch(err => { console.error('[Loom] Failed to load personas:', err); state.personas = []; }),
            loadDecisions().catch(err => { console.error('[Loom] Failed to load decisions:', err); state.decisions = []; }),
            loadSystemStatus().catch(err => { console.error('[Loom] Failed to load system status:', err); }),
            loadUsers().catch(err => { console.error('[Loom] Failed to load users:', err); state.users = []; }),
            loadAPIKeys().catch(err => { console.error('[Loom] Failed to load API keys:', err); state.apiKeys = []; }),
            loadMotivations().catch(err => { console.error('[Loom] Failed to load motivations:', err); })
        ]);
        await loadCeoBeads().catch(err => { console.error('[Loom] Failed to load CEO beads:', err); state.ceoBeads = []; });
        await loadActiveMeetings().catch(err => { console.error('[Loom] Failed to load active meetings:', err); state.activeMeetings = []; });
        await loadStatusBoardFeed().catch(err => { console.error('[Loom] Failed to load status board feed:', err); state.statusBoardFeed = []; });
        await loadOrgHealth().catch(err => { console.error('[Loom] Failed to load org health:', err); state.orgHealth = {}; });
        await loadReviewSummary().catch(err => { console.error('[Loom] Failed to load review summary:', err); state.reviewSummary = {}; });
        await loadEscalationQueue().catch(err => { console.error('[Loom] Failed to load escalation queue:', err); state.escalationQueue = []; });
        console.log('[Loom] Data loaded successfully:', {
            beads: state.beads?.length || 0,
            projects: state.projects?.length || 0,
            agents: state.agents?.length || 0
        });
        render();
        console.log('[Loom] render() completed');
    } catch (error) {
        console.error('[Loom] loadAll() failed:', error);
    }
}

// API calls
async function apiCall(endpoint, options = {}) {
    let autoFiledApiFailure = false;
    try {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };

        // Only add auth header if authentication is enabled and we have a token
        if (AUTH_ENABLED && !options.skipAuth && authToken) {
            headers.Authorization = `Bearer ${authToken}`;
        }

        const response = await fetch(`${API_BASE}${endpoint}`, {
            ...options,
            headers
        });

        // Backend is reachable — clear any connectivity banner
        if (backendDown) setBackendDown(false);

        if (!response.ok) {
            let message = 'API request failed';
            try {
                const error = await response.json();
                message = error.error || message;
            } catch {
                try {
                    const text = await response.text();
                    if (text) message = text;
                } catch {
                    // ignore
                }
            }
            // Only try to authenticate if auth is enabled
            if (AUTH_ENABLED && response.status === 401 && !options.skipAuth && !options.retryAuth) {
                await ensureAuth(true);
                return apiCall(endpoint, { ...options, retryAuth: true });
            }
            if (!options.skipAutoFile
                && typeof window !== 'undefined'
                && typeof window.fileApiBug === 'function') {
                window.fileApiBug({
                    endpoint,
                    method: options.method || 'GET',
                    status: response.status,
                    message,
                    response: message
                });
                autoFiledApiFailure = true;
            }
            throw new Error(message);
        }
        
        if (response.status === 204) {
            return null;
        }
        
        return await response.json();
    } catch (error) {
        const isNetworkError = !error.status && (
            (error.message || '').includes('Failed to fetch') ||
            (error.message || '').includes('NetworkError') ||
            (error.message || '').includes('Load failed') ||
            (error.message || '').includes('net::ERR_')
        );

        if (isNetworkError) {
            setBackendDown(true);
            throw error;
        }

        // Backend is reachable (got a real error response)
        if (backendDown) setBackendDown(false);

        if (!autoFiledApiFailure
            && !options.skipAutoFile
            && typeof window !== 'undefined'
            && typeof window.fileApiBug === 'function') {
            window.fileApiBug({
                endpoint,
                method: options.method || 'GET',
                status: 0,
                message: error.message || 'Network error',
                response: error.message || ''
            });
        }
        console.error('[Loom] API Error:', error);
        if (!options.suppressToast && !backendDown) {
            showToast(error.message || 'Request failed', 'error');
        }
        throw error;
    }
}

async function ensureAuth(forcePrompt = false) {
    // Skip authentication if it's disabled
    if (!AUTH_ENABLED) {
        return;
    }
    
    if (authCheckInFlight) return authCheckInFlight;
    authCheckInFlight = (async () => {
        if (!forcePrompt && authToken) {
            try {
                await apiCall('/auth/me', { skipAuth: false, suppressToast: true, retryAuth: true });
                return;
            } catch (err) {
                // Fall through to prompt.
            }
        }
        await showLoginModal();
    })().finally(() => {
        authCheckInFlight = null;
    });
    return authCheckInFlight;
}

async function showLoginModal() {
    if (loginInFlight) return loginInFlight;
    loginInFlight = (async () => {
        let loggedIn = false;
        while (!loggedIn) {
            const values = await formModal({
                title: 'Sign in',
                submitText: 'Sign in',
                fields: [
                    { id: 'username', label: 'Username', required: true, placeholder: 'admin' },
                    { id: 'password', label: 'Password', required: true, type: 'password', placeholder: 'Password' }
                ]
            });
            if (!values) {
                throw new Error('Login required');
            }
            try {
                const resp = await apiCall('/auth/login', {
                    method: 'POST',
                    body: JSON.stringify({
                        username: (values.username || '').trim(),
                        password: values.password || ''
                    }),
                    skipAuth: true
                });
                if (resp?.token) {
                    authToken = resp.token;
                    localStorage.setItem(AUTH_TOKEN_KEY, authToken);
                    showToast('Signed in', 'success');
                    loggedIn = true;
                } else {
                    throw new Error('Login failed');
                }
            } catch (err) {
                showToast(`Login failed: ${err.message || 'Unknown error'}`, 'error');
            }
        }
    })().finally(() => {
        loginInFlight = null;
    });
    return loginInFlight;
}

function scheduleReload(kind, delayMs = 150) {
    if (reloadTimers[kind]) return;
    reloadTimers[kind] = window.setTimeout(async () => {
        try {
            if (kind === 'beads') await loadBeads();
            if (kind === 'agents') await loadAgents();
            if (kind === 'projects') await loadProjects();
            if (kind === 'decisions') await loadDecisions();
            if (kind === 'status') await loadSystemStatus();
            if (!modalState.activeId) render();
        } catch (e) {
            // Errors are already surfaced via apiCall toasts.
        } finally {
            window.clearTimeout(reloadTimers[kind]);
            delete reloadTimers[kind];
        }
    }, delayMs);
}

function startEventStream() {
    if (typeof EventSource === 'undefined') return;

    try {
        const es = new EventSource(`${API_BASE}/events/stream`);

        es.addEventListener('connected', () => {
            eventStreamConnected = true;
        });

        const map = {
            'bead.created': ['beads', 'status'],
            'bead.assigned': ['beads', 'agents', 'status'],
            'bead.status_change': ['beads', 'status'],
            'bead.completed': ['beads', 'status'],
            'agent.spawned': ['agents', 'projects', 'status'],
            'agent.status_change': ['agents', 'status'],
            'agent.heartbeat': ['agents', 'status'],
            'agent.completed': ['agents', 'status'],
            'decision.created': ['decisions'],
            'decision.resolved': ['decisions'],
            'project.created': ['projects'],
            'project.updated': ['projects'],
            'project.deleted': ['projects'],
            'config.updated': ['projects', 'agents', 'status']
        };

        for (const [eventName, kinds] of Object.entries(map)) {
            es.addEventListener(eventName, () => {
                for (const k of kinds) scheduleReload(k);
            });
        }

        es.onerror = () => {
            eventStreamConnected = false;
            try {
                es.close();
            } catch {
                // ignore
            }
        };
    } catch {
        eventStreamConnected = false;
    }
}

function showToast(message, type = 'info', timeoutMs = 4500) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    window.setTimeout(() => {
        toast.remove();
    }, timeoutMs);
}

function setBusy(key, isBusy) {
    if (isBusy) busy.add(key);
    else busy.delete(key);
    render();
}

function isBusy(key) {
    return busy.has(key);
}

async function loadBeads() {
    state.beads = await apiCall('/beads');
}

async function loadCeoBeads() {
    const ceoIds = getCeoAgentIds();
    if (ceoIds.length === 0) {
        state.ceoBeads = [];
        return;
    }
    const query = encodeURIComponent(ceoIds.join(','));
    state.ceoBeads = await apiCall(`/beads?assigned_to=${query}`);
}

async function loadAgents() {
    state.agents = await apiCall('/agents');
}

async function loadProjects() {
    state.projects = await apiCall('/projects');
}

async function loadPersonas() {
    state.personas = await apiCall('/personas');
}

async function loadDecisions() {
    // Merge in-memory decisions (from DecisionManager) with persistent
    // decision-type beads from the bead store. The DecisionManager is
    // in-memory-only and loses state on restart; the bead store persists.
    const [inMemory, beadDecisions] = await Promise.all([
        apiCall('/decisions').catch(() => []),
        apiCall('/beads?type=decision').catch(() => [])
    ]);
    const seen = new Set((inMemory || []).map(d => d.id));
    const merged = [...(inMemory || [])];
    for (const b of (beadDecisions || [])) {
        if (!seen.has(b.id)) {
            merged.push({
                ...b,
                question: b.title || b.description || '',
                requester_id: (b.context && b.context.requester_id) || b.assigned_to || 'system',
                recommendation: (b.context && b.context.recommendation) || ''
            });
            seen.add(b.id);
        }
    }
    state.decisions = merged;
}

async function loadSystemStatus() {
    state.systemStatus = await apiCall('/system/status');
}

async function loadUsers() {
    try {
        state.users = await apiCall('/auth/users', { suppressToast: true, skipAutoFile: true });
    } catch (error) {
        state.users = [];
    }
}

async function loadAPIKeys() {
    try {
        state.apiKeys = await apiCall('/auth/api-keys', { suppressToast: true, skipAutoFile: true });
    } catch (error) {
        state.apiKeys = [];
    }
}

// Render functions
function render() {
    if (modalState.activeId) return; // Skip render while a modal is open
    if (window.pairState && window.pairState.open) return; // Skip render while pair panel is open
    renderSystemStatus();
    renderProjectViewer();
    renderKanban();
    renderAgents();
    renderProjects();
    renderPersonas();
    renderDecisions();
    renderCeoDashboard();
    renderCeoBeads();
    renderUsers();
    renderDiagrams();

    // New UI components
    renderActiveMeetings();
    renderStatusBoardFeed();
    renderOrgHealth();
    renderReviewSummary();
    renderEscalationQueue();
    if (typeof renderProjectsTable === 'function') {
        renderProjectsTable();
    }
    if (typeof renderConversationsView === 'function') {
        renderConversationsView();
    }
}

function renderProjectViewer() {
    const select = document.getElementById('project-view-select');
    const details = document.getElementById('project-view-details');
    if (!select || !details) return;

    const projects = state.projects || [];
    if (projects.length === 0) {
        select.innerHTML = '';
        details.innerHTML = renderEmptyState('No projects configured', 'Add a project to start tracking beads and agents.');
        return;
    }

    if (!uiState.project.selectedId) {
        uiState.project.selectedId = projects[0].id;
    }

    select.innerHTML = projects
        .map((p) => `<option value="${escapeHtml(p.id)}" ${p.id === uiState.project.selectedId ? 'selected' : ''}>${escapeHtml(p.name)} (${escapeHtml(p.id)})</option>`)
        .join('');

    const project = projects.find((p) => p.id === uiState.project.selectedId) || projects[0];
    uiState.project.selectedId = project.id;

    // Get agents assigned to this project
    const projectAgents = (state.agents || []).filter((a) => a.project_id === project.id);

    // Status badge class
    const statusClass = project.status === 'closed' ? 'priority-3' : '';

    details.innerHTML = `
        <div class="project-header" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; padding-bottom: 0.75rem; border-bottom: 1px solid var(--border-color);">
            <div>
                <span class="badge ${statusClass}" style="margin-right: 0.5rem;">${escapeHtml(project.status || 'open')}</span>
                <span class="small" style="color: var(--text-muted);">${escapeHtml(project.git_repo || '')} @ ${escapeHtml(project.branch || 'main')}</span>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button type="button" class="secondary" onclick="showProjectSettingsModal('${escapeHtml(project.id)}')" title="Project Settings" style="padding: 0.5rem 0.75rem;">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align: middle;">
                        <circle cx="12" cy="12" r="3"></circle>
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                    </svg>
                </button>
                <button type="button" class="danger" onclick="deleteProject('${escapeHtml(project.id)}')" title="Delete Project" style="padding: 0.5rem 0.75rem;">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align: middle;">
                        <polyline points="3 6 5 6 21 6"></polyline>
                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                    </svg>
                </button>
            </div>
        </div>
        <div style="display: grid; grid-template-columns: 1fr; gap: 1rem;">
            <div>
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                    <h4 style="color: var(--primary-color); margin: 0;">Assigned Agents (${projectAgents.length})</h4>
                    <button type="button" class="secondary" style="padding: 0.25rem 0.75rem; font-size: 0.85rem;" onclick="showAddAgentToProjectModal('${escapeHtml(project.id)}')">+ Add Agent</button>
                </div>
                <div id="project-agents-list" style="max-height: 200px; overflow-y: auto; border: 1px solid var(--border-color); border-radius: 4px;">
                    ${renderProjectAgentsList(projectAgents, project.id)}
                </div>
            </div>
        </div>
    `;

    const beads = (state.beads || []).filter((b) => b.project_id === project.id);
    const openBeads = beads.filter((b) => b.status === 'open');
    const inProgressBeads = beads.filter((b) => b.status === 'in_progress');
    const closedBeads = beads.filter((b) => b.status === 'closed');

    const openEl = document.getElementById('project-open-beads');
    const ipEl = document.getElementById('project-in-progress-beads');
    const closedEl = document.getElementById('project-closed-beads');
    if (openEl) {
        openEl.innerHTML = openBeads.length ? openBeads.map(renderBeadCard).join('') : renderEmptyState('No open beads', '');
    }
    if (ipEl) {
        ipEl.innerHTML = inProgressBeads.length ? inProgressBeads.map(renderBeadCard).join('') : renderEmptyState('Nothing in progress', '');
    }
    if (closedEl) {
        closedEl.innerHTML = closedBeads.length ? closedBeads.map(renderBeadCard).join('') : renderEmptyState('No closed beads', '');
    }

    // Keep the assignments board for backward compatibility
    const assignmentsEl = document.getElementById('project-agent-assignments');
    if (assignmentsEl) {
        assignmentsEl.style.display = 'none';
    }

    // D3 dashboard widgets
    if (typeof LoomCharts !== 'undefined') {
        // Agent status ring
        const ringEl = document.getElementById('d3-home-agent-ring');
        if (ringEl) {
            const statusCounts = {};
            (state.agents || []).forEach(function (a) {
                const s = a.status || 'unknown';
                statusCounts[s] = (statusCounts[s] || 0) + 1;
            });
            LoomCharts.statusRing(ringEl, statusCounts, { size: 140, centerLabel: 'agents' });
        }

        // Bead distribution treemap
        const tmEl = document.getElementById('d3-home-bead-treemap');
        if (tmEl) {
            const beadsByProject = {};
            (state.beads || []).forEach(function (b) {
                const pid = resolveProjectName(b.project_id) || b.project_id || 'unassigned';
                if (!beadsByProject[pid]) beadsByProject[pid] = { open: 0, in_progress: 0, closed: 0 };
                beadsByProject[pid][b.status] = (beadsByProject[pid][b.status] || 0) + 1;
            });
            const tmData = [];
            Object.keys(beadsByProject).forEach(function (p) {
                var counts = beadsByProject[p];
                if (counts.open) tmData.push({ label: p + ' open', value: counts.open, status: 'open' });
                if (counts.in_progress) tmData.push({ label: p + ' in progress', value: counts.in_progress, status: 'in_progress' });
                if (counts.closed) tmData.push({ label: p + ' closed', value: counts.closed, status: 'closed' });
            });
            if (tmData.length) LoomCharts.treemap(tmEl, tmData, { height: 140 });
        }
    }
}

// Org chart hierarchy order for sorting agents
const ORG_CHART_ORDER = [
    'ceo', 'cfo', 'product-manager', 'engineering-manager', 'project-manager',
    'qa-engineer', 'devops-engineer', 'code-reviewer', 'web-designer',
    'web-designer-engineer', 'documentation-manager', 'public-relations-manager',
    'decision-maker', 'housekeeping-bot'
];

function getOrgChartRank(agent) {
    const role = agent.role || extractRoleKey(agent.persona_name);
    const idx = ORG_CHART_ORDER.indexOf(role);
    return idx >= 0 ? idx : 999;
}

function extractRoleKey(personaName) {
    if (!personaName) return '';
    const parts = personaName.split('/');
    if (parts.length >= 2 && parts[0] === 'default') {
        return parts[1];
    } else if (parts.length >= 3 && parts[0] === 'projects') {
        return parts[2];
    }
    return parts[parts.length - 1];
}

function sortAgentsByOrgChart(agents) {
    return [...agents].sort((a, b) => getOrgChartRank(a) - getOrgChartRank(b));
}

function renderProjectAgentsList(agents, projectId) {
    if (agents.length === 0) {
        return `<div class="empty-state" style="padding: 1rem;"><p>No agents assigned yet.</p><p class="small">Add an agent from the org chart to get started.</p></div>`;
    }

    const sortedAgents = sortAgentsByOrgChart(agents);

    return sortedAgents.map((a) => {
        const bead = a.current_bead ? (state.beads || []).find((b) => b.id === a.current_bead) : null;
        const statusClass = a.status === 'working' ? 'working' : (a.status === 'blocked' ? 'blocked' : (a.status === 'paused' ? 'paused' : 'idle'));
        const roleName = extractRoleName(a.persona_name || a.name);
        
        return `
            <div class="agent-assignment-row" style="display: flex; justify-content: space-between; align-items: center; padding: 0.5rem; border-bottom: 1px solid var(--border-color); background: var(--card-bg);">
                <div style="flex: 1;">
                    <strong style="color: var(--text-color);">${escapeHtml(formatAgentDisplayName(a.name || roleName))}</strong>
                    <span class="badge ${statusClass}" style="margin-left: 0.5rem;">${escapeHtml(a.status || 'idle')}</span>
                    <div class="small" style="color: var(--text-muted);">
                        ${escapeHtml(a.persona_name || '')}
                        ${bead ? ` • Working on: ${escapeHtml(bead.title.substring(0, 30))}...` : ''}
                    </div>
                </div>
                <div style="display: flex; gap: 0.25rem;">
                    ${a.current_bead ? `<button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="viewAgentConversation('${escapeHtml(a.current_bead)}')" title="View conversation log">Log</button>` : ''}
                    <button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="showEditAgentModal('${escapeHtml(a.id)}')" title="Edit">Edit</button>
                    <button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="showCloneAgentModal('${escapeHtml(a.id)}')" title="Clone">Clone</button>
                    <button type="button" class="danger" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="removeAgentFromProject('${escapeHtml(projectId)}', '${escapeHtml(a.id)}')">Remove</button>
                </div>
            </div>
        `;
    }).join('');
}

function formatAgentDisplayName(name) {
    if (!name) return 'Agent';
    // Convert persona paths like "default/web-designer" to "Web Designer (Default)"
    if (name.includes('/')) {
        const parts = name.split('/');
        const role = parts[parts.length - 1].split('-').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
        const ns = parts[0].charAt(0).toUpperCase() + parts[0].slice(1);
        name = `${role} (${ns})`;
    }
    // Fix acronyms
    return name
        .replace(/\bCeo\b/gi, 'CEO')
        .replace(/\bCfo\b/gi, 'CFO');
}

function extractRoleName(personaName) {
    if (!personaName) return 'Agent';
    // Extract role from persona paths like "default/engineering-manager" or "projects/foo/engineering-manager/custom"
    const parts = personaName.split('/');
    let role = parts[parts.length - 1];
    if (parts.length >= 2 && parts[0] === 'default') {
        role = parts[1];
    } else if (parts.length >= 3 && parts[0] === 'projects') {
        role = parts[2];
    }
    // Convert kebab-case to Title Case, then fix acronyms
    const titleCase = role.split('-').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
    return formatAgentDisplayName(titleCase);
}

async function showAddAgentToProjectModal(projectId) {
    // Get available personas (the "org chart")
    const personas = state.personas || [];
    if (personas.length === 0) {
        showToast('No personas available. Add personas to the personas directory first.', 'error');
        return;
    }

    // Build persona options - only show default personas (the org chart), excluding templates
    const defaultPersonas = personas.filter(p => 
        p.name && 
        p.name.startsWith('default/') && 
        p.name !== 'templates'
    );
    const personaOptions = defaultPersonas.map(p => {
        const roleName = extractRoleName(p.name);
        return { value: p.name, label: `${roleName} (${p.name})` };
    });

    if (personaOptions.length === 0) {
        showToast('No default personas found in org chart. Add personas under personas/default/.', 'error');
        return;
    }

    try {
        const res = await formModal({
            title: 'Add Agent to Project',
            submitText: 'Create & Assign',
            fields: [
                {
                    id: 'persona_name',
                    label: 'Agent Role (from Org Chart)',
                    type: 'select',
                    required: true,
                    options: personaOptions
                },
                {
                    id: 'custom_name',
                    label: 'Custom Name (optional)',
                    type: 'text',
                    required: false,
                    placeholder: 'Leave empty for default name'
                }
            ]
        });

        if (!res) return;

        // Create the agent and assign to project
        const roleName = extractRoleName(res.persona_name);
        const agentName = res.custom_name || `${roleName} (Default)`;

        setBusy('addAgentToProject', true);
        await apiCall('/agents', {
            method: 'POST',
            body: JSON.stringify({
                name: agentName,
                persona_name: res.persona_name,
                project_id: projectId
            })
        });

        showToast(`Agent "${agentName}" created and assigned`, 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy('addAgentToProject', false);
    }
}

async function removeAgentFromProject(projectId, agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    const agentName = agent ? (agent.name || agentId) : agentId;

    const ok = await confirmModal({
        title: 'Remove Agent?',
        body: `Remove "${agentName}" from this project? This will stop the agent.`,
        confirmText: 'Remove',
        cancelText: 'Cancel',
        danger: true
    });

    if (!ok) return;

    try {
        setBusy(`removeAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, { method: 'DELETE' });
        showToast('Agent removed', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`removeAgent:${agentId}`, false);
    }
}

async function showEditAgentModal(agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    if (!agent) {
        showToast('Agent not found', 'error');
        return;
    }

    const persona = agent.persona || {};
    const displayName = formatAgentDisplayName(agent.name || extractRoleName(agent.persona_name));

    try {
        const res = await formModal({
            title: `Edit Agent: ${displayName}`,
            submitText: 'Save Changes',
            fields: [
                {
                    id: 'name',
                    label: 'Agent Name',
                    type: 'text',
                    required: true,
                    value: agent.name || ''
                },
                {
                    id: 'mission',
                    label: 'Mission (Job Description)',
                    type: 'textarea',
                    required: false,
                    value: persona.mission || '',
                    placeholder: 'Describe what this agent does...'
                },
                {
                    id: 'character',
                    label: 'Character',
                    type: 'textarea',
                    required: false,
                    value: persona.character || '',
                    placeholder: 'Describe the agent\'s character...'
                },
                {
                    id: 'tone',
                    label: 'Tone',
                    type: 'textarea',
                    required: false,
                    value: persona.tone || '',
                    placeholder: 'e.g., Professional, friendly, direct...'
                },
                {
                    id: 'autonomy_level',
                    label: 'Autonomy Level',
                    type: 'select',
                    required: true,
                    value: persona.autonomy_level || 'semi',
                    options: [
                        { value: 'full', label: 'Full - Can make all non-P0 decisions' },
                        { value: 'semi', label: 'Semi - Can make routine decisions' },
                        { value: 'supervised', label: 'Supervised - Requires approval for all decisions' }
                    ]
                }
            ]
        });

        if (!res) return;

        // Update agent persona via API
        setBusy(`editAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, {
            method: 'PUT',
            body: JSON.stringify({
                name: res.name,
                persona: {
                    ...persona,
                    mission: res.mission,
                    character: res.character,
                    tone: res.tone,
                    autonomy_level: res.autonomy_level
                }
            })
        });
        showToast('Agent updated', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`editAgent:${agentId}`, false);
    }
}

async function showCloneAgentModal(agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    if (!agent) {
        showToast('Agent not found', 'error');
        return;
    }

    const displayName = formatAgentDisplayName(agent.name || extractRoleName(agent.persona_name));
    const roleName = agent.role || extractRoleKey(agent.persona_name);

    try {
        const res = await formModal({
            title: `Clone Agent: ${displayName}`,
            submitText: 'Create Clone',
            fields: [
                {
                    id: 'new_name',
                    label: 'New Agent Name',
                    type: 'text',
                    required: true,
                    value: `${agent.name || roleName} (Copy)`,
                    placeholder: 'Enter name for the cloned agent'
                },
                {
                    id: 'new_persona_name',
                    label: 'New Persona Name',
                    type: 'text',
                    required: true,
                    value: `${roleName}-copy`,
                    placeholder: 'e.g., custom-reviewer'
                },
                {
                    id: 'replace_original',
                    label: 'Replace Original',
                    type: 'checkbox',
                    required: false,
                    value: false,
                    description: 'Stop the original agent after cloning'
                }
            ]
        });

        if (!res) return;

        setBusy(`cloneAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}/clone`, {
            method: 'POST',
            body: JSON.stringify({
                new_agent_name: res.new_name,
                new_persona_name: res.new_persona_name,
                replace: res.replace_original || false
            })
        });
        showToast('Agent cloned successfully', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`cloneAgent:${agentId}`, false);
    }
}

async function showProjectSettingsModal(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    if (!project) {
        showToast('Project not found', 'error');
        return;
    }

    try {
        const usesSSH = project.git_auth_method === 'ssh'
            || (project.git_repo || '').trim().startsWith('git@')
            || (project.git_repo || '').trim().startsWith('ssh://');
        let gitKeyValue = '';
        let gitKeyPlaceholder = 'Not available';
        let gitKeyDescription = 'SSH git auth is required to display the deploy key.';
        if (usesSSH) {
            try {
                const keyRes = await apiCall(`/projects/${projectId}/git-key`, {
                    suppressToast: true,
                    skipAutoFile: true
                });
                gitKeyValue = keyRes?.public_key || '';
                gitKeyPlaceholder = gitKeyValue ? '' : 'No key returned';
                gitKeyDescription = 'Add this as a write-enabled deploy key in your git host.';
            } catch (error) {
                gitKeyDescription = 'Unable to fetch the git key.';
            }
        }
        const res = await formModal({
            title: `Project Settings: ${project.name}`,
            submitText: 'Save Settings',
            fields: [
                {
                    id: 'name',
                    label: 'Project Name',
                    type: 'text',
                    required: true,
                    value: project.name || ''
                },
                {
                    id: 'git_repo',
                    label: 'GitHub Repository',
                    type: 'text',
                    required: true,
                    value: project.git_repo || '',
                    placeholder: 'https://github.com/org/repo'
                },
                {
                    id: 'git_key',
                    label: 'Project Git Key (Deploy Key)',
                    type: 'textarea',
                    required: false,
                    value: gitKeyValue,
                    placeholder: gitKeyPlaceholder,
                    description: gitKeyDescription,
                    readonly: true
                },
                {
                    id: 'branch',
                    label: 'Branch',
                    type: 'text',
                    required: true,
                    value: project.branch || 'main'
                },
                {
                    id: 'beads_path',
                    label: 'Beads Path',
                    type: 'text',
                    required: false,
                    value: project.beads_path || '.beads',
                    placeholder: '.beads'
                },
                {
                    id: 'status',
                    label: 'Status',
                    type: 'select',
                    required: true,
                    value: project.status || 'open',
                    options: [
                        { value: 'open', label: 'Open' },
                        { value: 'closed', label: 'Closed' },
                        { value: 'reopened', label: 'Reopened' }
                    ]
                },
                {
                    id: 'is_perpetual',
                    label: 'Perpetual Project',
                    type: 'select',
                    required: false,
                    value: project.is_perpetual ? 'true' : 'false',
                    options: [
                        { value: 'false', label: 'No - Project can be closed' },
                        { value: 'true', label: 'Yes - Project never closes' }
                    ]
                },
                {
                    id: 'is_sticky',
                    label: 'Sticky Project',
                    type: 'select',
                    required: false,
                    value: project.is_sticky ? 'true' : 'false',
                    options: [
                        { value: 'false', label: 'No' },
                        { value: 'true', label: 'Yes - Auto-added on startup' }
                    ]
                }
            ]
        });

        if (!res) return;

        const payload = {
            name: res.name,
            git_repo: res.git_repo,
            branch: res.branch,
            beads_path: res.beads_path || '.beads',
            status: res.status,
            is_perpetual: res.is_perpetual === 'true',
            is_sticky: res.is_sticky === 'true'
        };

        setBusy('projectSettings', true);
        await apiCall(`/projects/${projectId}`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });

        showToast('Project settings saved', 'success');
        await loadProjects();
        render();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy('projectSettings', false);
    }
}

async function assignAgentToProject(projectId) {
    try {
        const res = await formModal({
            title: 'Assign agent to project',
            submitText: 'Assign',
            fields: [{ id: 'agent_id', label: 'Agent ID', type: 'text', required: true, placeholder: 'agent-123' }]
        });
        if (!res) return;

        await apiCall(`/projects/${projectId}/agents`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: res.agent_id, action: 'assign' })
        });

        showToast('Agent assigned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    }
}

async function unassignAgentFromProject(projectId, agentId) {
    try {
        await apiCall(`/projects/${projectId}/agents`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: agentId, action: 'unassign' })
        });

        showToast('Agent unassigned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    }
}

function renderSystemStatus() {
    const el = document.getElementById('system-status');
    if (!el) return;

    const s = state.systemStatus;
    if (!s) {
        el.innerHTML = '';
        return;
    }

    const badge = s.state === 'active' ? `<span class="badge">active</span>` : `<span class="badge">parked</span>`;
    const reason = s.reason ? escapeHtml(s.reason) : '';
    el.innerHTML = `${badge} ${reason}`;
}

function renderKanban() {
    // Populate project filter dropdown
    const projectSelect = document.getElementById('bead-project');
    if (projectSelect) {
        const projects = state.projects || [];
        const currentVal = uiState.bead.project || 'all';
        const opts = '<option value="all">All Projects</option>' +
            projects.map(p => `<option value="${escapeHtml(p.id)}"${p.id === currentVal ? ' selected' : ''}>${escapeHtml(p.name)}</option>`).join('');
        if (projectSelect.innerHTML !== opts) projectSelect.innerHTML = opts;
        projectSelect.value = currentVal;
    }

    const filtered = getFilteredBeads();
    const openBeads = filtered.filter((b) => b.status === 'open');
    const blockedBeads = filtered.filter((b) => b.status === 'blocked');
    const inProgressBeads = filtered.filter((b) => b.status === 'in_progress');
    const blockedBeads = filtered.filter((b) => b.status === 'blocked');
    const closedBeads = filtered.filter((b) => b.status === 'closed');

    const openEl = document.getElementById('open-beads');
    const blockedEl = document.getElementById('blocked-beads');
    const ipEl = document.getElementById('in-progress-beads');
    const blockedEl = document.getElementById('blocked-beads');
    const closedEl = document.getElementById('closed-beads');
    if (!openEl || !ipEl || !blockedEl || !closedEl) return;

    openEl.innerHTML =
        openBeads.length > 0
            ? openBeads.map(renderBeadCard).join('')
            : renderEmptyState('No open beads', 'Create a bead via the API or bd CLI, then it will show up here.');
    blockedEl.innerHTML =
        blockedBeads.length > 0
            ? blockedBeads.map(renderBeadCard).join('')
            : renderEmptyState('No blocked beads', 'Blocked beads will appear here when work is waiting on dependencies or intervention.');
    ipEl.innerHTML =
        inProgressBeads.length > 0
            ? inProgressBeads.map(renderBeadCard).join('')
            : renderEmptyState('Nothing in progress', 'Claim a bead to move it into progress.');
    blockedEl.innerHTML =
        blockedBeads.length > 0
            ? blockedBeads.map(renderBeadCard).join('')
            : renderEmptyState('No blocked beads', 'Beads that encounter blockers will appear here.');
    closedEl.innerHTML =
        closedBeads.length > 0
            ? closedBeads.map(renderBeadCard).join('')
            : renderEmptyState('No closed beads yet', 'Completed beads will appear here.');
}
function renderAgents() {
    const q = (uiState.agent.search || '').trim().toLowerCase();
    
    // Helper function to get bead title by ID
    function getBeadTitle(beadId) {
        if (!beadId) return '';
        const bead = state.beads.find(b => b.id === beadId);
        return bead ? bead.title : beadId;
    }
    
    // Helper function to get performance grade color
    function getGradeColor(grade) {
        const gradeColors = {
            'A': '#16a34a',  // green
            'B': '#2563eb',  // blue
            'C': '#eab308',  // yellow
            'D': '#ea580c',  // orange
            'F': '#dc2626'   // red
        };
        return gradeColors[grade] || '#94a3b8';
    }
    
    // Helper function to get status color (from d3-charts.js STATUS_COLORS)
    function getStatusColor(status) {
        const statusColors = {
            'working': '#16a34a',
            'idle': '#2563eb',
            'paused': '#d97706',
            'error': '#dc2626',
            'blocked': '#dc2626',
            'healthy': '#16a34a',
            'active': '#16a34a',
            'pending': '#d97706',
            'failed': '#dc2626',
            'open': '#2563eb',
            'in_progress': '#7c3aed',
            'closed': '#64748b',
            'done': '#059669'
        };
        return statusColors[(status || '').toLowerCase()] || '#94a3b8';
    }
    
    // Filter out templates agent (it does nothing for now)
    const visibleAgents = state.agents.filter((a) => a.persona_name !== 'templates');
    const agents = q
        ? visibleAgents.filter((a) => {
              const hay = `${a.name || ''} ${a.persona_name || ''}`.toLowerCase();
              return hay.includes(q);
          })
        : visibleAgents;

    const html = agents.map(agent => {
        const statusClass = agent.status;
        const displayName = agent.display_name || formatAgentDisplayName(agent.name || agent.persona_name || agent.id);
        const statusColor = getStatusColor(agent.status);
        const beadTitle = agent.current_bead ? getBeadTitle(agent.current_bead) : '';
        
        return `
            <div class="agent-card ${statusClass}">
                <div class="agent-header">
                    <span class="agent-name">${escapeHtml(displayName)}</span>
                    <span class="agent-status ${statusClass}" style="display: flex; align-items: center; gap: 0.5rem;">
                        <span style="display: inline-block; width: 8px; height: 8px; border-radius: 50%; background-color: ${statusColor};"></span>
                        ${agent.status}
                    </span>
                </div>
                ${agent.motivation_summary ? `<div style="font-style: italic; color: var(--text-muted); font-size: 0.9rem; margin: 0.5rem 0;">${escapeHtml(agent.motivation_summary)}</div>` : ''}
                <div>
                    <strong>Persona:</strong> ${escapeHtml(agent.persona_name)}<br>
                    <strong>Project:</strong> ${escapeHtml(resolveProjectName(agent.project_id))}<br>
                    ${agent.current_bead ? `<strong>Working on:</strong> ${escapeHtml(beadTitle)}` : ''}
                </div>
                ${agent.performance_grade ? `
                <div style="margin-top: 0.5rem; display: flex; align-items: center; gap: 0.5rem;">
                    <strong>Grade:</strong>
                    <span class="badge" style="background-color: ${getGradeColor(agent.performance_grade)}; color: white; font-weight: bold; padding: 0.25rem 0.5rem; border-radius: 3px;">
                        ${escapeHtml(agent.performance_grade)}
                    </span>
                </div>
                ` : ''}
                <div style="margin-top: 1rem;">
                    ${agent.current_bead ? `<button class="secondary" onclick="viewAgentConversation('${escapeHtml(agent.current_bead)}')" title="View conversation log">Log</button>` : ''}
                    <button class="secondary" onclick="cloneAgentPersona('${agent.id}')" ${isBusy(`cloneAgent:${agent.id}`) ? 'disabled' : ''}>${isBusy(`cloneAgent:${agent.id}`) ? 'Cloning…' : 'Clone Persona'}</button>
                    <button class="danger" onclick="stopAgent('${agent.id}')" ${isBusy(`stopAgent:${agent.id}`) ? 'disabled' : ''}>${isBusy(`stopAgent:${agent.id}`) ? 'Stopping…' : 'Stop Agent'}</button>
                </div>
            </div>
        `;
    }).join('');

    const agentListEl = document.getElementById('agent-list');
    if (!agentListEl) return;
    agentListEl.innerHTML =
        agents.length > 0
            ? html
            : renderEmptyState(
                  'No active agents',
                  'Spawn an agent to start working on beads.',
                  '<button type="button" class="secondary" onclick="showSpawnAgentModal()">Spawn your first agent</button>'
              );

    // D3 agent overview charts
    if (typeof LoomCharts !== 'undefined') {
        const ringEl = document.getElementById('d3-agents-status-ring');
        if (ringEl) {
            const counts = {};
            agents.forEach(function (a) { var s = a.status || 'unknown'; counts[s] = (counts[s] || 0) + 1; });
            LoomCharts.statusRing(ringEl, counts, { size: 150, centerLabel: 'agents' });
        }
        const workEl = document.getElementById('d3-agents-workload-donut');
        if (workEl) {
            const workData = {};
            agents.forEach(function (a) {
                var name = formatAgentDisplayName(a.name || a.persona_name || a.id);
                workData[name] = (workData[name] || 0) + 1;
            });
            LoomCharts.donut(workEl, workData, { centerLabel: 'agents', size: 180 });
        }
    }
}

function renderProjects() {
    const html = state.projects.map(project => `
        <div class="project-card">
            <h3>${escapeHtml(project.name)}</h3>
            <div>
                <strong>Branch:</strong> ${escapeHtml(project.branch)}<br>
                <strong>Repo:</strong> ${escapeHtml(project.git_repo)}<br>
                <strong>Agents:</strong> ${project.agents ? project.agents.length : 0}
            </div>
            <div style="margin-top: 0.75rem; display: flex; gap: 0.5rem; flex-wrap: wrap;">
                <button type="button" class="secondary" onclick="viewProject('${escapeHtml(project.id)}')">View</button>
                <button type="button" class="secondary" onclick="showEditProjectModal('${escapeHtml(project.id)}')">Edit</button>
                <button type="button" class="danger" onclick="deleteProject('${escapeHtml(project.id)}')">Delete</button>
            </div>
        </div>
    `).join('');

    const projectList = document.getElementById('project-list');
    if (!projectList) return;
    projectList.innerHTML =
        html || renderEmptyState('No projects configured', 'Add a project to get started.', '<button type="button" onclick="showBootstrapProjectModal()">New Project</button>');
}

function viewProject(projectId) {
    uiState.project.selectedId = projectId;
    if (typeof window.activateViewTab === 'function') {
        window.activateViewTab('project-viewer');
    }
    location.hash = '#project-viewer';
    render();
}

function projectFormFields(project = {}) {
    return [
        { id: 'name', label: 'Name', type: 'text', required: true, value: project.name || '' },
        { id: 'git_repo', label: 'Git repo', type: 'text', required: true, value: project.git_repo || '' },
        { id: 'branch', label: 'Branch', type: 'text', required: true, value: project.branch || 'main' },
        { id: 'beads_path', label: 'Beads path', type: 'text', required: false, value: project.beads_path || '.beads' },
        {
            id: 'is_perpetual',
            label: 'Perpetual project',
            type: 'select',
            required: false,
            value: project.is_perpetual ? 'true' : 'false',
            options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ]
        },
        {
            id: 'is_sticky',
            label: 'Sticky project',
            type: 'select',
            required: false,
            value: project.is_sticky ? 'true' : 'false',
            options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ]
        },
        {
            id: 'git_strategy',
            label: 'Git strategy',
            type: 'select',
            value: project.git_strategy || 'direct',
            options: [
                { value: 'direct', label: 'Direct to branch' },
                { value: 'branch-pr', label: 'Feature branch + PR' }
            ]
        }
    ];
}

function parseBool(value) {
    return value === 'true' || value === '1' || value === 'yes';
}

function buildProjectPayload(data) {
    const payload = {
        name: (data.name || '').trim(),
        git_repo: (data.git_repo || '').trim(),
        branch: (data.branch || '').trim(),
        beads_path: (data.beads_path || '').trim(),
        is_perpetual: parseBool(data.is_perpetual || 'false'),
        is_sticky: parseBool(data.is_sticky || 'false'),
        git_strategy: (data.git_strategy || 'direct').trim()
    };

    if (!payload.name) delete payload.name;
    if (!payload.git_repo) delete payload.git_repo;
    if (!payload.branch) delete payload.branch;
    if (!payload.beads_path) delete payload.beads_path;

    return payload;
}

async function showCreateProjectModal() {
    try {
        const res = await formModal({
            title: 'Add project',
            submitText: 'Create',
            fields: projectFormFields()
        });
        if (!res) return;

        await apiCall('/projects', {
            method: 'POST',
            body: JSON.stringify(buildProjectPayload(res))
        });

        showToast('Project created', 'success');
        await loadProjects();
        render();
    } catch (e) {
        // handled
    }
}

async function showEditProjectModal(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    if (!project) return;

    try {
        const res = await formModal({
            title: `Edit project ${project.name}`,
            submitText: 'Save',
            fields: projectFormFields(project)
        });
        if (!res) return;

        await apiCall(`/projects/${projectId}`, {
            method: 'PUT',
            body: JSON.stringify(buildProjectPayload(res))
        });

        showToast('Project updated', 'success');
        await loadProjects();
        render();
    } catch (e) {
        // handled
    }
}

async function deleteProject(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    const ok = await confirmModal({
        title: 'Delete project?',
        body: `This will delete project ${project ? project.name : projectId}.`,
        confirmText: 'Delete',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        await apiCall(`/projects/${projectId}`, { method: 'DELETE' });
        showToast('Project deleted', 'success');
        await loadProjects();
        if (uiState.project.selectedId === projectId) {
            uiState.project.selectedId = '';
        }
        render();
    } catch (e) {
        // handled
    }
}

// Streaming utilities
function createStreamingRequest(endpoint, body, options = {}) {
    const { onChunk, onComplete, onError, useStreaming = true } = options;
    
    if (!useStreaming || typeof EventSource === 'undefined') {
        // Fallback to non-streaming
        return apiCall(endpoint, {
            method: 'POST',
            body: JSON.stringify(body)
        });
    }

    return new Promise((resolve, reject) => {
        // Use streaming endpoint
        const streamEndpoint = endpoint + '/stream';
        
        fetch(`${API_BASE}${streamEndpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...(authToken ? { 'Authorization': `Bearer ${authToken}` } : {})
            },
            body: JSON.stringify(body)
        }).then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';
            let fullContent = '';

            function processText(text) {
                buffer += text;
                const lines = buffer.split('\n');
                buffer = lines.pop() || ''; // Keep incomplete line in buffer

                for (const line of lines) {
                    if (!line.trim()) continue;
                    
                    if (line.startsWith('event: ')) {
                        const eventType = line.substring(7).trim();
                        continue; // Event type will be followed by data line
                    }

                    if (line.startsWith('data: ')) {
                        const data = line.substring(6);
                        
                        try {
                            const chunk = JSON.parse(data);
                            
                            // Handle different event types
                            if (chunk.message) {
                                // Connected or done event
                                if (chunk.message.includes('complete')) {
                                    if (onComplete) onComplete(fullContent);
                                    resolve({ response: fullContent });
                                }
                                continue;
                            }

                            if (chunk.error) {
                                if (onError) onError(chunk.error);
                                reject(new Error(chunk.error));
                                return;
                            }

                            // Extract content from streaming chunk
                            if (chunk.choices && chunk.choices[0]) {
                                const delta = chunk.choices[0].delta;
                                if (delta && delta.content) {
                                    fullContent += delta.content;
                                    if (onChunk) onChunk(delta.content, fullContent);
                                }
                            }
                        } catch (e) {
                            // Ignore JSON parse errors for non-JSON data lines
                        }
                    }
                }
            }

            function pump() {
                reader.read().then(({ done, value }) => {
                    if (done) {
                        if (onComplete) onComplete(fullContent);
                        resolve({ response: fullContent });
                        return;
                    }

                    processText(decoder.decode(value, { stream: true }));
                    pump();
                }).catch(error => {
                    // Better error message for mid-stream failures
                    let errorMsg = error.message || 'Stream interrupted';
                    if (errorMsg.includes('network') || errorMsg.includes('NetworkError')) {
                        errorMsg = fullContent 
                            ? 'Connection lost during response (partial content received)' 
                            : 'Network error - check provider connection';
                    }
                    if (onError) onError(errorMsg);
                    reject(new Error(errorMsg));
                });
            }

            pump();
        }).catch(error => {
            // Provide more helpful error messages for common failures
            let errorMsg = error.message || 'Unknown error';
            if (errorMsg.includes('Failed to fetch') || errorMsg.includes('NetworkError')) {
                errorMsg = 'network error';
            } else if (errorMsg.includes('aborted')) {
                errorMsg = 'request was cancelled';
            }
            if (onError) onError(errorMsg);
            reject(new Error(errorMsg));
        });
    });
}

async function sendReplQuery() {
    const input = document.getElementById('repl-input');
    const responseEl = document.getElementById('repl-response');
    const sendBtn = document.getElementById('repl-send');
    const streamToggle = document.getElementById('repl-stream-toggle');
    if (!input || !responseEl || !sendBtn) return;

    const message = (input.value || '').trim();
    if (!message) {
        showToast('Enter a question first.', 'error');
        return;
    }

    const useStreaming = streamToggle ? streamToggle.checked : true;

    try {
        setBusy('repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = useStreaming ? 'Streaming…' : 'Sending…';
        responseEl.textContent = '';
        responseEl.classList.add('streaming');

        const requestBody = {
            messages: [
                { role: 'system', content: 'You are the CEO of Loom. Respond concisely and helpfully.' },
                { role: 'user', content: message }
            ]
        };

        if (useStreaming) {
            // Use streaming
            await createStreamingRequest('/chat/completions', requestBody, {
                useStreaming: true,
                onChunk: (chunk, fullContent) => {
                    responseEl.textContent = fullContent;
                    // Auto-scroll to bottom
                    responseEl.scrollTop = responseEl.scrollHeight;
                },
                onComplete: (fullContent) => {
                    responseEl.classList.remove('streaming');
                    responseEl.classList.add('complete');
                },
                onError: (error) => {
                    responseEl.classList.remove('streaming');
                    responseEl.textContent += `\n\n[Error: ${error}]`;
                }
            });
        } else {
            // Use non-streaming
            const res = await apiCall('/chat/completions', {
                method: 'POST',
                body: JSON.stringify(requestBody)
            });

            if (res.choices && res.choices[0] && res.choices[0].message) {
                responseEl.textContent = res.choices[0].message.content || 'No response';
            } else {
                responseEl.textContent = 'No response returned.';
            }
            responseEl.classList.remove('streaming');
            responseEl.classList.add('complete');
        }
    } catch (e) {
        responseEl.classList.remove('streaming');
        responseEl.textContent = `Request failed: ${e.message || 'Unknown error'}`;
    } finally {
        setBusy('repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

// Assign agent to project from CEO REPL
async function assignAgentFromCeoRepl() {
    const agentSelect = document.getElementById('ceo-repl-agent-select');
    const projectSelect = document.getElementById('ceo-repl-project-select');
    
    const agentId = agentSelect ? agentSelect.value : '';
    const projectId = projectSelect ? projectSelect.value : '';
    
    if (!agentId) {
        showToast('Select an agent to assign.', 'error');
        return;
    }
    
    if (!projectId) {
        showToast('Select a project to assign the agent to.', 'error');
        return;
    }
    
    const agent = (state.agents || []).find(a => a.id === agentId);
    if (!agent) {
        showToast('Agent not found.', 'error');
        return;
    }
    
    const project = (state.projects || []).find(p => p.id === projectId);
    if (!project) {
        showToast('Project not found.', 'error');
        return;
    }
    
    try {
        // Update the agent's project_id
        await apiCall(`/agents/${encodeURIComponent(agentId)}`, {
            method: 'PUT',
            body: JSON.stringify({ project_id: projectId })
        });
        
        const agentName = agent.name || agent.persona_name || agentId;
        const projectName = project.name || projectId;
        showToast(`Assigned ${agentName} to ${projectName}`, 'success');
        
        // Refresh data to reflect the change
        loadAll();
    } catch (error) {
        // Error already handled by apiCall
    }
}

// CEO REPL — supports "agent: message" routing and general queries
async function sendCeoReplQuery() {
    const input = document.getElementById('ceo-repl-input');
    const responseEl = document.getElementById('ceo-repl-response');
    const sendBtn = document.getElementById('ceo-repl-send');
    const agentSelect = document.getElementById('ceo-repl-agent-select');
    if (!input || !responseEl || !sendBtn) return;

    const message = (input.value || '').trim();
    if (!message) {
        showToast('Enter a question first.', 'error');
        return;
    }

    // Check if an agent is selected from the dropdown
    const selectedAgentId = agentSelect ? agentSelect.value : '';
const selectedProjectId = projectSelect ? projectSelect.value : '';
    if (selectedAgentId) {
        // Find the selected agent and dispatch to them
        const selectedAgent = (state.agents || []).find(a => a.id === selectedAgentId);
        if (selectedAgent) {
            return ceoReplDispatchToAgentById(selectedAgent, message, responseEl, sendBtn);
        }
    }

    // Parse "agent: message" pattern for direct agent dispatch (legacy support)
    const agentMatch = message.match(/^([a-zA-Z][a-zA-Z0-9 _-]+):\s+(.+)/s);
    if (agentMatch) {
        const agentQuery = agentMatch[1].trim().toLowerCase();
        const taskMessage = agentMatch[2].trim();
        return ceoReplDispatchToAgent(agentQuery, taskMessage, responseEl, sendBtn);
    }

    // General query — use streaming chat completion
    return ceoReplStreamQuery(message, responseEl, sendBtn);
}

// Dispatch a task to a specific agent selected from the dropdown
async function ceoReplDispatchToAgentById(agent, taskMessage, responseEl, sendBtn) {
    const projectId = selectedProjectId || agent.project_id || uiState.project.selectedId || ((state.projects || [])[0] || {}).id || '';
    if (!projectId) {
        responseEl.innerHTML = '<span style="color:var(--danger-color)">No project selected. Select a project first.</span>';
        return;
    }

    try {
        setBusy('ceo-repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = 'Dispatching…';
        responseEl.textContent = '';

        // Create a bead with the task
        const bead = await apiCall('/beads', {
            method: 'POST',
            skipAutoFile: true,
            body: JSON.stringify({
                title: taskMessage.substring(0, 100),
                description: taskMessage,
                type: 'task',
                priority: 1,
                project_id: projectId
            })
        });

        // Assign to the selected agent
        await apiCall(`/beads/${bead.id}`, {
            method: 'PATCH',
            skipAutoFile: true,
            body: JSON.stringify({ assigned_to: agent.id })
        });

        const agentDisplay = agent.name || agent.role || agent.persona_name || agent.id;
        responseEl.innerHTML = `<strong style="color:var(--success-color)">Task dispatched!</strong>\n\nBead: <code>${escapeHtml(bead.id)}</code>\nAssigned to: <strong>${escapeHtml(agentDisplay)}</strong>\nTitle: ${escapeHtml(bead.title)}\n\nThe dispatcher will pick this up on the next cycle.`;

        // Refresh data
        setTimeout(() => loadAll(), 1000);
    } catch (e) {
        responseEl.innerHTML = `<span style="color:var(--danger-color)">Failed to dispatch: ${escapeHtml(e.message)}</span>`;
    } finally {
        setBusy('ceo-repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

// Dispatch a task to a specific agent via "agent: message" syntax
async function ceoReplDispatchToAgent(agentQuery, taskMessage, responseEl, sendBtn) {
    const projectId = uiState.project.selectedId || ((state.projects || [])[0] || {}).id || '';
    if (!projectId) {
        responseEl.innerHTML = '<span style="color:var(--danger-color)">No project selected. Select a project first.</span>';
        return;
    }

    // Find matching agent in the current project
    const projectAgents = (state.agents || []).filter(a => a.project_id === projectId);
    const matchedAgent = projectAgents.find(a => {
        const name = (a.name || '').toLowerCase();
        const role = (a.role || '').toLowerCase();
        const persona = (a.persona_name || '').toLowerCase();
        return name.includes(agentQuery) || role.includes(agentQuery) || persona.includes(agentQuery);
    });

    if (!matchedAgent) {
        const available = projectAgents.map(a => {
            const display = a.name || a.role || a.persona_name;
            return `  - ${display} (${a.status})`;
        }).join('\n');
        responseEl.innerHTML = `<span style="color:var(--danger-color)">No agent matching "${escapeHtml(agentQuery)}" found in this project.</span>\n\n<strong>Available agents:</strong>\n${escapeHtml(available || '  (none)')}`;
        return;
    }

    try {
        setBusy('ceo-repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = 'Dispatching…';
        responseEl.textContent = '';

        // Create a bead with the task
        const bead = await apiCall('/beads', {
            method: 'POST',
            skipAutoFile: true,
            body: JSON.stringify({
                title: taskMessage.substring(0, 100),
                description: taskMessage,
                type: 'task',
                priority: 1,
                project_id: projectId
            })
        });

        // Assign to the matched agent
        await apiCall(`/beads/${bead.id}`, {
            method: 'PATCH',
            skipAutoFile: true,
            body: JSON.stringify({ assigned_to: matchedAgent.id })
        });

        const agentDisplay = matchedAgent.name || matchedAgent.role || matchedAgent.persona_name;
        responseEl.innerHTML = `<strong style="color:var(--success-color)">Task dispatched!</strong>\n\nBead: <code>${escapeHtml(bead.id)}</code>\nAssigned to: <strong>${escapeHtml(agentDisplay)}</strong>\nTitle: ${escapeHtml(bead.title)}\n\nThe dispatcher will pick this up on the next cycle.`;

        // Refresh data
        setTimeout(() => loadAll(), 1000);
    } catch (e) {
        responseEl.innerHTML = `<span style="color:var(--danger-color)">Failed to dispatch: ${escapeHtml(e.message)}</span>`;
    } finally {
        setBusy('ceo-repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

// Stream a general CEO query via chat completion
async function ceoReplStreamQuery(message, responseEl, sendBtn) {
    try {
        setBusy('ceo-repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = 'Streaming…';
        responseEl.textContent = '';
        responseEl.classList.add('streaming');

        // Build context about current project state
        const projectId = uiState.project.selectedId || ((state.projects || [])[0] || {}).id || '';
        const project = state.projects.find(p => p.id === projectId);
        const projectAgents = (state.agents || []).filter(a => a.project_id === projectId);
        const agentSummary = projectAgents.map(a => `${a.name || a.role} (${a.status}${a.current_bead ? ', working on ' + a.current_bead : ''})`).join(', ');
        const beadCounts = {
            open: (state.beads || []).filter(b => b.status === 'open' && b.project_id === projectId).length,
            in_progress: (state.beads || []).filter(b => b.status === 'in_progress' && b.project_id === projectId).length,
        };

        const systemPrompt = `You are the CEO dashboard assistant for Loom, an autonomous agent orchestration system.
Current project: ${project ? project.name : 'unknown'} (${projectId})
Agents: ${agentSummary || 'none'}
Beads: ${beadCounts.open} open, ${beadCounts.in_progress} in progress

To dispatch a task to an agent, the user should type: agent_name: task description
For example: "code reviewer: Review the authentication module for security issues"

Answer questions concisely about the system state, agents, and beads.`;

        const requestBody = {
            messages: [
                { role: 'system', content: systemPrompt },
                { role: 'user', content: message }
            ]
        };

        await createStreamingRequest('/chat/completions', requestBody, {
            useStreaming: true,
            onChunk: (chunk, fullContent) => {
                responseEl.textContent = fullContent;
                responseEl.scrollTop = responseEl.scrollHeight;
            },
            onComplete: () => {
                responseEl.classList.remove('streaming');
                responseEl.classList.add('complete');
            },
            onError: (error) => {
                responseEl.classList.remove('streaming');
                responseEl.textContent += '\n\n[Error: ' + error + ']';
            }
        });
    } catch (e) {
        responseEl.classList.remove('streaming');
        responseEl.textContent = 'Request failed: ' + (e.message || 'Unknown error');
    } finally {
        setBusy('ceo-repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

function renderPersonas() {
    // Filter out templates persona (it does nothing for now)
    const visiblePersonas = state.personas.filter(p => p.name !== 'templates');
    const html = visiblePersonas.map(persona => `
        <button type="button" class="persona-card" onclick="editPersona('${escapeHtml(persona.name)}')" aria-label="Edit persona: ${escapeHtml(persona.name)}">
            <h3>🎭 ${escapeHtml(persona.name)}</h3>
            <div>
                <strong>Autonomy:</strong> ${escapeHtml(persona.autonomy_level || 'semi')}<br>
                <strong>Character:</strong> ${escapeHtml((persona.character || '').substring(0, 100))}...
            </div>
        </button>
    `).join('');
    
    const personaListEl = document.getElementById('persona-list');
    if (!personaListEl) return;
    personaListEl.innerHTML =
        html || renderEmptyState('No personas available', 'Add personas under ./personas to populate this list.');
}

async function cloneAgentPersona(agentId) {
    const agent = state.agents.find((a) => a.id === agentId);
    if (!agent) return;

    try {
        const res = await formModal({
            title: 'Clone agent persona',
            submitText: 'Clone',
            fields: [
                { id: 'new_persona_name', label: 'New persona name', type: 'text', required: true, placeholder: 'custom-qa-engineer' },
                { id: 'new_agent_name', label: 'New agent name (optional)', type: 'text', required: false, placeholder: `${agent.name}-custom` },
                { id: 'source_persona', label: 'Source persona (optional)', type: 'text', required: false, placeholder: 'default/qa-engineer' }
            ]
        });
        if (!res) return;

        const replace = await confirmModal({
            title: 'Replace current agent?',
            body: 'Replace this agent with the cloned persona? (Recommended to avoid duplicates.)',
            confirmText: 'Replace',
            cancelText: 'Keep both'
        });

        setBusy(`cloneAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}/clone`, {
            method: 'POST',
            body: JSON.stringify({
                new_persona_name: res.new_persona_name,
                new_agent_name: res.new_agent_name || '',
                source_persona: res.source_persona || '',
                replace: replace
            })
        });

        showToast('Persona cloned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`cloneAgent:${agentId}`, false);
    }
}

function renderDecisions() {
    const items = (state.decisions || []).filter(d => d.status !== 'closed');
    const filterValue = (uiState.decision && uiState.decision.filter) || 'all';
    
    // Separate decisions by type
    const humanEscalations = items.filter(d => d.context && d.context.requires_human === 'true');
    const agentHandled = items.filter(d => !(d.context && d.context.requires_human === 'true'));
    
    // Render human escalation section
    const humanHtml = humanEscalations.map(decision => {
        const p0Class = decision.priority === 0 ? 'p0' : '';
        const claimKey = `claimDecision:${decision.id}`;
        const decideKey = `makeDecision:${decision.id}`;
        const displayText = decision.question || decision.title || decision.description || '(no description)';
        const requester = decision.requester_id || decision.assigned_to || 'system';
        const escalationReason = decision.context && decision.context.escalation_reason
            ? `<br><strong>Reason:</strong> ${escapeHtml(decision.context.escalation_reason)}`
            : '';
        return `
            <div class="decision-card ${p0Class}">
                <div class="decision-question">${escapeHtml(displayText)}</div>
                <div>
                    <strong>Priority:</strong> P${decision.priority}
                    <strong>Status:</strong> ${escapeHtml(decision.status || 'open')}
                    <br><strong>Requester:</strong> ${escapeHtml(requester)}
                    ${decision.recommendation ? `<br><strong>Recommendation:</strong> ${escapeHtml(decision.recommendation)}` : ''}
                    ${escalationReason}
                </div>
                <div class="decision-actions">
                    <button class="secondary" onclick="viewBead('${decision.id}')">View</button>
                    <button onclick="claimDecision('${decision.id}')" ${isBusy(claimKey) ? 'disabled' : ''}>${isBusy(claimKey) ? 'Claiming...' : 'Claim'}</button>
                    ${decision.status === 'in_progress' ? `<button class="secondary" onclick="makeDecision('${decision.id}')" ${isBusy(decideKey) ? 'disabled' : ''}>${isBusy(decideKey) ? 'Submitting...' : 'Decide'}</button>` : ''}
                </div>
            </div>
        `;
    }).join('');
    
    // Render agent-handled section
    const agentHtml = agentHandled.map(decision => {
        const p0Class = decision.priority === 0 ? 'p0' : '';
        const displayText = decision.question || decision.title || decision.description || '(no description)';
        const assignedAgent = decision.assigned_to || 'system';
        const agentObj = (state.agents || []).find(a => a.id === assignedAgent);
        const agentName = agentObj ? (agentObj.display_name || agentObj.name || assignedAgent) : assignedAgent;
        return `
            <div class="decision-card ${p0Class}">
                <div class="decision-question">${escapeHtml(displayText)}</div>
                <div>
                    <strong>Priority:</strong> P${decision.priority}
                    <strong>Status:</strong> ${escapeHtml(decision.status || 'open')}
                    <br><strong>Handled by:</strong> ${escapeHtml(agentName)}
                    ${decision.recommendation ? `<br><strong>Recommendation:</strong> ${escapeHtml(decision.recommendation)}` : ''}
                </div>
                <div class="decision-actions">
                    <button class="secondary" onclick="viewBead('${decision.id}')">View</button>
                </div>
            </div>
        `;
    }).join('');
    
    // Render recently closed decisions
    const closedDecisions = (state.decisions || []).filter(d => d.status === 'closed').slice(0, 10);
    const closedHtml = closedDecisions.map(decision => {
        const displayText = decision.question || decision.title || decision.description || '(no description)';
        const assignedAgent = decision.assigned_to || 'system';
        const agentObj = (state.agents || []).find(a => a.id === assignedAgent);
        const agentName = agentObj ? (agentObj.display_name || agentObj.name || assignedAgent) : assignedAgent;
        const rationale = decision.context && decision.context.rationale ? decision.context.rationale : '';
        return `
            <div class="decision-card" style="opacity: 0.8;">
                <div class="decision-question">${escapeHtml(displayText)}</div>
                <div>
                    <strong>Decided by:</strong> ${escapeHtml(agentName)}
                    ${rationale ? `<br><strong>Rationale:</strong> ${escapeHtml(rationale)}` : ''}
                </div>
                <div class="decision-actions">
                    <button class="secondary" onclick="viewBead('${decision.id}')">View</button>
                </div>
            </div>
        `;
    }).join('');
 
    const decisionListEl = document.getElementById('decision-list');
    if (!decisionListEl) return;
    
    let content = '';
    if (humanEscalations.length > 0) {
        content += `<div style="margin-bottom: 2rem;"><h3 style="color: var(--primary-color); margin-top: 0;">Requires Human Review (${humanEscalations.length})</h3>${humanHtml}</div>`;
    }
    if (agentHandled.length > 0) {
        content += `<div style="margin-bottom: 2rem;"><h3 style="color: var(--primary-color); margin-top: 0;">In Progress by Agent (${agentHandled.length})</h3>${agentHtml}</div>`;
    }
    if (closedDecisions.length > 0) {
        content += `<div style="margin-bottom: 2rem;"><h3 style="color: var(--primary-color); margin-top: 0;">Recently Decided (${closedDecisions.length})</h3>${closedHtml}</div>`;
    }
    
    decisionListEl.innerHTML = content || renderEmptyState('No decisions pending human review', 'Agents are handling decisions autonomously.');
}
function renderCeoDashboard() {
    const container = document.getElementById('ceo-dashboard-summary');
    if (!container) return;

    const projects = state.projects || [];
    const beads = state.beads || [];
    const agents = state.agents || [];
    const decisions = state.decisions || [];

    // Populate the CEO REPL project dropdown for agent assignment
if (projectSelect) {
    const currentValue = projectSelect.value;
    
    // Clear existing options except the first one
    while (projectSelect.options.length > 1) {
        projectSelect.remove(1);
    }
    
    // Add all projects
    for (const project of (state.projects || [])) {
        const option = document.createElement('option');
        option.value = project.id;
        option.textContent = project.name || project.id;
        projectSelect.appendChild(option);
    }
    
    // Restore previous selection if still valid
    if (currentValue && Array.from(projectSelect.options).some(o => o.value === currentValue)) {
        projectSelect.value = currentValue;
    }
}

// Populate the CEO REPL agent dropdown with ALL agents grouped by project
    const agentSelect = document.getElementById('ceo-repl-agent-select');
    if (agentSelect) {
        const currentValue = agentSelect.value;
        
        // Clear all existing options and optgroups except the first option (General Query)
        while (agentSelect.children.length > 1) {
            agentSelect.removeChild(agentSelect.lastChild);
        }
        
        // Group agents by project
        const agentsByProject = {};
        const unassignedAgents = [];
        
        for (const agent of agents) {
            if (agent.project_id) {
                if (!agentsByProject[agent.project_id]) {
                    agentsByProject[agent.project_id] = [];
                }
                agentsByProject[agent.project_id].push(agent);
            } else {
                unassignedAgents.push(agent);
            }
        }
        
        // Add agents grouped by project using optgroup elements
        for (const project of projects) {
            const projectAgents = agentsByProject[project.id] || [];
            if (projectAgents.length === 0) continue;
            
            const optgroup = document.createElement('optgroup');
            optgroup.label = project.name || project.id;
            
            for (const agent of projectAgents) {
                const option = document.createElement('option');
                option.value = agent.id;
                const displayName = agent.name || agent.role || agent.persona_name || agent.id;
                const status = agent.status ? ` (${agent.status})` : '';
                option.textContent = displayName + status;
                optgroup.appendChild(option);
            }
            
            agentSelect.appendChild(optgroup);
        }
        
        // Add unassigned agents if any
        if (unassignedAgents.length > 0) {
            const optgroup = document.createElement('optgroup');
            optgroup.label = 'Unassigned';
            
            for (const agent of unassignedAgents) {
                const option = document.createElement('option');
                option.value = agent.id;
                const displayName = agent.name || agent.role || agent.persona_name || agent.id;
                const status = agent.status ? ` (${agent.status})` : '';
                option.textContent = displayName + status;
                optgroup.appendChild(option);
            }
            
            agentSelect.appendChild(optgroup);
        }
        
        // Restore previous selection if still valid
        if (currentValue) {
            const allOptions = agentSelect.querySelectorAll('option');
            for (const opt of allOptions) {
                if (opt.value === currentValue) {
                    agentSelect.value = currentValue;
                    break;
                }
            }
        }
    }
    // Populate the CEO REPL project dropdown for agent assignment
    const projectSelect = document.getElementById('ceo-repl-project-select');
    if (projectSelect) {
        const currentValue = projectSelect.value;
        
        // Clear existing options except the first one
        while (projectSelect.options.length > 1) {
            projectSelect.remove(1);
        }
        
        // Add all projects
        for (const project of (state.projects || [])) {
            const option = document.createElement('option');
            option.value = project.id;
            option.textContent = project.name || project.id;
            projectSelect.appendChild(option);
        }
        
        // Restore previous selection if still valid
        if (currentValue && Array.from(projectSelect.options).some(o => o.value === currentValue)) {
            projectSelect.value = currentValue;
        }
    }

    const beadCounts = {
        open: beads.filter((b) => b.status === 'open').length,
        in_progress: beads.filter((b) => b.status === 'in_progress').length,
        blocked: beads.filter((b) => b.status === 'blocked').length,
        closed: beads.filter((b) => b.status === 'closed').length
    };

    const agentCounts = {
        idle: agents.filter((a) => a.status === 'idle').length,
        working: agents.filter((a) => a.status === 'working').length,
        paused: agents.filter((a) => a.status === 'paused').length
    };

    const decisionCounts = {
        open: decisions.filter((d) => d.status === 'open').length,
        in_progress: decisions.filter((d) => d.status === 'in_progress').length,
        closed: decisions.filter((d) => d.status === 'closed').length
    };

    container.innerHTML = `
        <div class="ceo-dashboard-card">
            <h3>Projects</h3>
            <div class="small">Total: ${projects.length}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Beads</h3>
            <div class="small">Open: ${beadCounts.open} • In progress: ${beadCounts.in_progress} • Blocked: ${beadCounts.blocked} • Closed: ${beadCounts.closed}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Agents</h3>
            <div class="small">Idle: ${agentCounts.idle} • Working: ${agentCounts.working} • Paused: ${agentCounts.paused}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Decisions</h3>
            <div class="small">Open: ${decisionCounts.open} • In progress: ${decisionCounts.in_progress} • Closed: ${decisionCounts.closed}</div>
        </div>
    `;
}

function viewDecision(decisionId) {
    const d = state.decisions.find((x) => x.id === decisionId);
    if (!d) return;

    const body = `
        <div>
            <div style="margin-bottom: 0.5rem;"><span class="badge priority-${d.priority}">P${d.priority}</span> <span class="badge">decision</span> <span class="badge">${escapeHtml(d.status || '')}</span></div>
            <div><strong>ID:</strong> ${escapeHtml(d.id)}</div>
            <div><strong>Requester:</strong> ${escapeHtml(d.requester_id || '')}</div>
            ${d.recommendation ? `<div style="margin-top: 0.5rem;"><strong>Recommendation:</strong> ${escapeHtml(d.recommendation)}</div>` : ''}
            ${Array.isArray(d.options) && d.options.length > 0 ? `<div style="margin-top: 0.5rem;"><strong>Options:</strong> ${d.options.map((o) => `<span class="badge">${escapeHtml(String(o))}</span>`).join(' ')}</div>` : ''}
            <div style="margin-top: 1rem; white-space: pre-wrap;">${escapeHtml(d.question || '')}</div>
        </div>
    `;

    openAppModal({
        title: 'Decision details',
        bodyHtml: body,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() },
            {
                label: 'Claim',
                onClick: async () => {
                    closeAppModal();
                    await claimDecision(decisionId);
                }
            }
        ]
    });
}

// Actions

async function showChangePasswordModal() {
    const values = await formModal({
        title: 'Change Password',
        submitText: 'Change',
        fields: [
            { id: 'old_password', label: 'Current Password', required: true, type: 'password', placeholder: 'Enter current password' },
            { id: 'new_password', label: 'New Password', required: true, type: 'password', placeholder: 'Enter new password (min 8 chars)' },
            { id: 'confirm_password', label: 'Confirm Password', required: true, type: 'password', placeholder: 'Re-enter new password' }
        ]
    });
    
    if (!values) return;
    
    const oldPwd = (values.old_password || '').trim();
    const newPwd = (values.new_password || '').trim();
    const confirmPwd = (values.confirm_password || '').trim();
    
    if (!oldPwd || !newPwd || !confirmPwd) {
        showToast('All fields are required', 'error');
        return;
    }
    
    if (newPwd.length < 8) {
        showToast('New password must be at least 8 characters', 'error');
        return;
    }
    
    if (newPwd !== confirmPwd) {
        showToast('Passwords do not match', 'error');
        return;
    }
    
    if (oldPwd === newPwd) {
        showToast('New password must be different from current password', 'error');
        return;
    }
    
    try {
        await apiCall('/auth/change-password', {
            method: 'POST',
            body: JSON.stringify({
                old_password: oldPwd,
                new_password: newPwd
            })
        });
        showToast('Password changed successfully', 'success');
    } catch (err) {
        showToast(`Failed to change password: ${err.message || 'Unknown error'}`, 'error');
    }
}







function showSpawnAgentModal() {
    // Populate persona and project dropdowns
    const personaSelect = document.getElementById('agent-persona');
    const projectSelect = document.getElementById('agent-project');
    
    personaSelect.innerHTML = state.personas.map(p => 
        `<option value="${escapeHtml(p.name)}">${escapeHtml(p.name)}</option>`
    ).join('');
    
    projectSelect.innerHTML = state.projects.map(p => 
        `<option value="${p.id}">${escapeHtml(p.name)}</option>`
    ).join('');

    openModal('spawn-agent-modal', { initialFocusSelector: '#agent-name' });
}

function closeSpawnAgentModal() {
    closeModal('spawn-agent-modal');
}

document.getElementById('spawn-agent-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    
    const formData = new FormData(e.target);
    const data = {
        name: formData.get('name'),
        persona_name: formData.get('persona_name'),
        project_id: formData.get('project_id')
    };
    
    try {
        setBusy('spawnAgent', true);

        const submitBtn = e.target.querySelector('button[type="submit"]');
        const prevText = submitBtn?.textContent;
        if (submitBtn) {
            submitBtn.disabled = true;
            submitBtn.textContent = 'Spawning…';
        }

        await apiCall('/agents', {
            method: 'POST',
            body: JSON.stringify(data)
        });

        showToast('Agent spawned', 'success');
        closeSpawnAgentModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        const submitBtn = e.target.querySelector('button[type="submit"]');
        if (submitBtn) {
            submitBtn.disabled = false;
            submitBtn.textContent = submitBtn.textContent === 'Spawning…' ? 'Spawn Agent' : submitBtn.textContent;
        }
        setBusy('spawnAgent', false);
    }
});

async function stopAgent(agentId) {
    const ok = await confirmModal({
        title: 'Stop agent?',
        body: 'This will stop the agent and release its file locks.',
        confirmText: 'Stop agent',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;
    
    try {
        setBusy(`stopAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, {
            method: 'DELETE'
        });

        showToast('Agent stopped', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`stopAgent:${agentId}`, false);
    }
}

function viewBead(beadId) {
    const bead = state.beads.find(b => b.id === beadId);
    if (!bead) return;

    const statusClass = getCondensedBeadStatusClass(bead);
    const tagsValue = Array.isArray(bead.tags) ? bead.tags.join(', ') : '';
    const blockedByValue = Array.isArray(bead.blocked_by) ? bead.blocked_by.join(', ') : '';
    const blocksValue = Array.isArray(bead.blocks) ? bead.blocks.join(', ') : '';
    const relatedToValue = Array.isArray(bead.related_to) ? bead.related_to.join(', ') : '';
    const childrenValue = Array.isArray(bead.children) ? bead.children.join(', ') : '';
    const contextJson = bead.context ? JSON.stringify(bead.context, null, 2) : '';
    const dueDateValue = bead.due_date ? new Date(bead.due_date).toISOString().slice(0, 16) : '';

    const statusOptions = ['open', 'in_progress', 'blocked', 'closed', 'deferred', 'ready', 'tombstone', 'pinned']
        .map(s => `<option value="${s}"${bead.status === s ? ' selected' : ''}>${s}</option>`)
        .join('');
    const priorityOptions = [0, 1, 2, 3, 4]
        .map(p => `<option value="${p}"${bead.priority === p ? ' selected' : ''}>P${p}</option>`)
        .join('');

    const availableAgents = (state.agents || []).filter(a => a.status !== 'terminated');
    const agentOptions = '<option value="">-- select agent --</option>' +
        availableAgents.map(a => {
            const display = a.name || a.role || a.persona_name || a.id;
            return `<option value="${escapeHtml(a.id)}"${bead.assigned_to === a.id ? ' selected' : ''}>${escapeHtml(display)} (${escapeHtml(a.status)})</option>`;
        }).join('');

    const body = `
        <div class="bead-modal-viewer bead-condensed ${statusClass}">
            <div class="bead-modal-header">
                <code>${escapeHtml(bead.id)}</code>
                <span class="badge priority-${bead.priority}">P${bead.priority}</span>
                <span class="badge">${escapeHtml(bead.type)}</span>
                <span class="badge">${escapeHtml(bead.status)}</span>
            </div>

            <div class="bead-modal-assign">
                <strong>Agent Assignment</strong>
                <select id="bead-modal-agent" style="width:100%;margin:0.5rem 0;">${agentOptions}</select>
                <button type="button" id="bead-modal-dispatch-btn" class="secondary" style="width:100%;">Assign &amp; Dispatch</button>
            </div>

            <div class="bead-modal-fields">
                <label for="bead-modal-title">Title</label>
                <input id="bead-modal-title" type="text" value="${escapeHtml(bead.title || '')}" />

                <label for="bead-modal-type">Type</label>
                <input id="bead-modal-type" type="text" value="${escapeHtml(bead.type || '')}" />

                <label for="bead-modal-status">Status</label>
                <select id="bead-modal-status">${statusOptions}</select>

                <label for="bead-modal-priority">Priority</label>
                <select id="bead-modal-priority">${priorityOptions}</select>

                <label for="bead-modal-project">Project ID</label>
                <input id="bead-modal-project" type="text" value="${escapeHtml(bead.project_id || '')}" />

                <label for="bead-modal-parent">Parent</label>
                <input id="bead-modal-parent" type="text" value="${escapeHtml(bead.parent || '')}" placeholder="parent bead ID" />

                <label for="bead-modal-tags">Tags</label>
                <input id="bead-modal-tags" type="text" value="${escapeHtml(tagsValue)}" placeholder="comma-separated" />

                <label for="bead-modal-blocked-by">Blocked by</label>
                <input id="bead-modal-blocked-by" type="text" value="${escapeHtml(blockedByValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-blocks">Blocks</label>
                <input id="bead-modal-blocks" type="text" value="${escapeHtml(blocksValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-related">Related to</label>
                <input id="bead-modal-related" type="text" value="${escapeHtml(relatedToValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-children">Children</label>
                <input id="bead-modal-children" type="text" value="${escapeHtml(childrenValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-due">Due date</label>
                <input id="bead-modal-due" type="datetime-local" value="${dueDateValue}" />

                <label for="bead-modal-milestone">Milestone ID</label>
                <input id="bead-modal-milestone" type="text" value="${escapeHtml(bead.milestone_id || '')}" />

                <label for="bead-modal-estimate">Est. minutes</label>
                <input id="bead-modal-estimate" type="number" value="${bead.estimated_time || 0}" min="0" />

                <label for="bead-modal-desc">Description</label>
                <textarea id="bead-modal-desc" rows="4">${escapeHtml(bead.description || '')}</textarea>

                <label for="bead-modal-context">Context (JSON)</label>
                <textarea id="bead-modal-context" rows="3">${escapeHtml(contextJson)}</textarea>
            </div>

            <details class="bead-modal-meta">
                <summary>Timestamps</summary>
                <div><strong>Created:</strong> ${bead.created_at ? new Date(bead.created_at).toLocaleString() : 'unknown'}</div>
                <div><strong>Updated:</strong> ${bead.updated_at ? new Date(bead.updated_at).toLocaleString() : 'unknown'}</div>
                <div><strong>Closed:</strong> ${bead.closed_at ? new Date(bead.closed_at).toLocaleString() : '<em>n/a</em>'}</div>
            </details>
        </div>
    `;

    openAppModal({
        title: 'Bead Details',
        bodyHtml: body,
        actions: [
            { label: 'Save Changes', variant: '', onClick: () => saveBeadFromModal(bead.id) },
            { label: 'Pair', variant: 'secondary', onClick: () => {
                const agentId = (document.getElementById('bead-modal-agent') || {}).value || '';
                closeAppModal();
                if (typeof openPairPanel === 'function') openPairPanel(bead.id, agentId);
            }},
            { label: 'Redispatch', variant: 'secondary', onClick: () => redispatchBead(bead.id) },
            { label: 'Close Bead', variant: 'secondary', onClick: () => closeBeadFromModal(bead.id) },
            { label: 'Dismiss', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });

    // Wire up dispatch button after modal is in the DOM
    const dispatchBtn = document.getElementById('bead-modal-dispatch-btn');
    if (dispatchBtn) {
        dispatchBtn.addEventListener('click', () => dispatchBeadFromModal(bead.id));
    }
}

async function saveBeadFromModal(beadId) {
    const val = (id) => (document.getElementById(id) || {}).value || '';
    const parseList = (v) => String(v).split(',').map(s => s.trim()).filter(s => s.length > 0);

    const contextRaw = val('bead-modal-context').trim();
    let parsedContext = {};
    if (contextRaw) {
        try {
            parsedContext = JSON.parse(contextRaw);
        } catch (e) {
            showToast('Context must be valid JSON.', 'error');
            return;
        }
    }

    const dueRaw = val('bead-modal-due');
    const payload = {
        title: val('bead-modal-title'),
        type: val('bead-modal-type'),
        status: val('bead-modal-status'),
        priority: Number(val('bead-modal-priority')),
        project_id: val('bead-modal-project'),
        parent: val('bead-modal-parent'),
        tags: parseList(val('bead-modal-tags')),
        blocked_by: parseList(val('bead-modal-blocked-by')),
        blocks: parseList(val('bead-modal-blocks')),
        related_to: parseList(val('bead-modal-related')),
        children: parseList(val('bead-modal-children')),
        milestone_id: val('bead-modal-milestone'),
        estimated_time: Number(val('bead-modal-estimate')) || 0,
        description: val('bead-modal-desc'),
        context: parsedContext
    };
    if (dueRaw) payload.due_date = new Date(dueRaw).toISOString();

    try {
        setBusy(`editBead:${beadId}`, true);
        await saveBeadUpdate(beadId, payload, { successMessage: 'Bead updated' });
        closeAppModal();
    } catch (error) {
        // Error already handled by saveBeadUpdate
    } finally {
        setBusy(`editBead:${beadId}`, false);
    }
}

async function dispatchBeadFromModal(beadId) {
    const agentSelect = document.getElementById('bead-modal-agent');
    const agentId = agentSelect ? agentSelect.value : '';
    if (!agentId) {
        showToast('Select an agent first.', 'error');
        return;
    }

    const ok = await confirmModal({
        title: 'Assign & Dispatch?',
        body: `Claim bead ${beadId} for agent ${agentId}? This sets status to in_progress and kicks the workflow engine.`,
        confirmText: 'Dispatch',
        cancelText: 'Cancel'
    });
    if (!ok) return;

    try {
        setBusy(`dispatchBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/claim`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: agentId })
        });
        showToast('Bead dispatched to ' + agentId, 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`dispatchBead:${beadId}`, false);
    }
}

async function closeBeadFromModal(beadId) {
    const ok = await confirmModal({
        title: 'Close bead?',
        body: `Close bead ${beadId}? This will mark it as closed.`,
        confirmText: 'Close',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        setBusy(`closeBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { status: 'closed' }, { successMessage: 'Bead closed' });
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`closeBead:${beadId}`, false);
    }
}

async function dispatchBeadToAgent(beadId) {
    const select = document.getElementById(`ceo-dispatch-${beadId}`);
    const agentId = select ? select.value : '';
    if (!agentId) {
        showToast('Select an agent to dispatch.', 'error');
        return;
    }

    const ok = await confirmModal({
        title: 'Dispatch bead?',
        body: `Assign bead ${beadId} to ${agentId}?`,
        confirmText: 'Dispatch',
        cancelText: 'Cancel'
    });
    if (!ok) return;

    try {
        setBusy(`dispatchBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { assigned_to: agentId, status: 'in_progress' }, { successMessage: 'Bead dispatched' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`dispatchBead:${beadId}`, false);
    }
}

async function closeBeadFromCeo(beadId) {
    const ok = await confirmModal({
        title: 'Close bead?',
        body: `Close bead ${beadId}? This will mark it as closed.`,
        confirmText: 'Close bead',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        setBusy(`closeBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { status: 'closed' }, { successMessage: 'Bead closed' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`closeBead:${beadId}`, false);
    }
}

async function showBeadCommentModal(beadId) {
    try {
        const res = await formModal({
            title: 'Add bead comment',
            submitText: 'Add comment',
            fields: [
                { id: 'comment', label: 'Comment', type: 'textarea', required: true, placeholder: 'Add context or instructions for this bead.' }
            ]
        });
        if (!res || !res.comment) return;

        setBusy(`commentBead:${beadId}`, true);
        const bead = await apiCall(`/beads/${beadId}`);
        const context = bead.context || {};
        const comments = parseBeadComments(context);

        comments.push({
            id: `comment-${Date.now()}`,
            author: 'ceo',
            timestamp: new Date().toISOString(),
            comment: String(res.comment || '').trim()
        });

        context.comments = JSON.stringify(comments);
        context.last_comment_by = 'ceo';
        context.last_comment_at = new Date().toISOString();

        await saveBeadUpdate(beadId, { context: context }, { successMessage: 'Comment added' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`commentBead:${beadId}`, false);
    }
}

function showAllBeadCommentsModal(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const comments = parseBeadComments(bead.context || {});
    const bodyHtml = comments.length
        ? `
            <div class="ceo-comment-list">
                ${comments
                    .map((c) => {
                        const author = escapeHtml(c.author || 'unknown');
                        const timestamp = escapeHtml((c.timestamp || '').substring(0, 19).replace('T', ' '));
                        const text = escapeHtml(c.comment || '');
                        return `
                            <div class="ceo-comment-item">
                                <div class="small"><strong>${author}</strong> · ${timestamp}</div>
                                <div class="ceo-comment-text">${text}</div>
                            </div>
                        `;
                    })
                    .join('')}
            </div>
        `
        : '<p class="small">No comments yet.</p>';

    openAppModal({
        title: `Comments for ${bead.title}`,
        bodyHtml: bodyHtml,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });
}

function openBeadDetails(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const bodyHtml = renderBeadDetailsHtml(bead);
    openAppModal({
        title: `Bead details: ${bead.title}`,
        bodyHtml,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });
}

function renderBeadDetailsHtml(bead) {
    const tags = Array.isArray(bead.tags) ? bead.tags : [];
    const blockedBy = Array.isArray(bead.blocked_by) ? bead.blocked_by : [];
    const blocks = Array.isArray(bead.blocks) ? bead.blocks : [];
    const relatedTo = Array.isArray(bead.related_to) ? bead.related_to : [];
    const children = Array.isArray(bead.children) ? bead.children : [];
    const context = bead.context ? JSON.stringify(bead.context, null, 2) : '';

    return `
        <div class="bead-details">
            <div class="bead-details-actions">
                <button type="button" class="icon-inline-button" onclick="openEditBeadModal('${escapeHtml(bead.id)}')" aria-label="Edit bead">✏️</button>
            </div>
            <div><strong>ID:</strong> ${escapeHtml(bead.id || '')}</div>
            <div><strong>Title:</strong> ${escapeHtml(bead.title || '')}</div>
            <div><strong>Type:</strong> ${escapeHtml(bead.type || '')}</div>
            <div><strong>Status:</strong> ${escapeHtml(bead.status || '')}</div>
            <div><strong>Priority:</strong> ${escapeHtml(String(bead.priority ?? ''))}</div>
            <div><strong>Project:</strong> ${escapeHtml(bead.project_id || '')}</div>
            <div><strong>Assigned to:</strong> ${escapeHtml(bead.assigned_to || '')}</div>
            <div><strong>Parent:</strong> ${escapeHtml(bead.parent || '')}</div>
            <div><strong>Tags:</strong> ${escapeHtml(tags.join(', '))}</div>
            <div><strong>Blocked by:</strong> ${escapeHtml(blockedBy.join(', '))}</div>
            <div><strong>Blocks:</strong> ${escapeHtml(blocks.join(', '))}</div>
            <div><strong>Related to:</strong> ${escapeHtml(relatedTo.join(', '))}</div>
            <div><strong>Children:</strong> ${escapeHtml(children.join(', '))}</div>
            <div><strong>Created at:</strong> ${escapeHtml(String(bead.created_at || ''))}</div>
            <div><strong>Updated at:</strong> ${escapeHtml(String(bead.updated_at || ''))}</div>
            <div><strong>Closed at:</strong> ${escapeHtml(String(bead.closed_at || ''))}</div>
            <div style="margin-top: 0.5rem;"><strong>Description:</strong></div>
            <div style="white-space: pre-wrap;">${escapeHtml(bead.description || '')}</div>
            <div style="margin-top: 0.5rem;"><strong>Context:</strong></div>
            <pre class="small" style="white-space: pre-wrap;">${escapeHtml(context)}</pre>
        </div>
    `;
}

function getCondensedBeadStatusClass(bead) {
    const status = String(bead.status || '').toLowerCase();
    const blocks = Array.isArray(bead.blocks) ? bead.blocks : [];
    const classes = [];
    if (status) classes.push(`bead-status-${status}`);
    if (blocks.length > 0) classes.push('bead-status-blocks');
    return classes.join(' ');
}

async function openEditBeadModal(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const tagsValue = Array.isArray(bead.tags) ? bead.tags.join(', ') : '';
    const blockedByValue = Array.isArray(bead.blocked_by) ? bead.blocked_by.join(', ') : '';
    const blocksValue = Array.isArray(bead.blocks) ? bead.blocks.join(', ') : '';
    const relatedToValue = Array.isArray(bead.related_to) ? bead.related_to.join(', ') : '';
    const childrenValue = Array.isArray(bead.children) ? bead.children.join(', ') : '';
    const contextValue = bead.context ? JSON.stringify(bead.context, null, 2) : '';

    const res = await formModal({
        title: 'Edit bead',
        submitText: 'Save',
        cancelText: 'Cancel',
        fields: [
            { id: 'title', label: 'Title', type: 'text', required: true, value: bead.title || '' },
            { id: 'type', label: 'Type', type: 'text', required: true, value: bead.type || '' },
            { id: 'status', label: 'Status', type: 'select', required: true, value: bead.status || 'open', options: [
                { value: 'open', label: 'open' },
                { value: 'in_progress', label: 'in_progress' },
                { value: 'blocked', label: 'blocked' },
                { value: 'closed', label: 'closed' }
            ]},
            { id: 'priority', label: 'Priority', type: 'number', required: true, value: bead.priority ?? 2 },
            { id: 'project_id', label: 'Project ID', type: 'text', required: true, value: bead.project_id || '' },
            { id: 'assigned_to', label: 'Assigned to', type: 'text', required: false, value: bead.assigned_to || '' },
            { id: 'parent', label: 'Parent', type: 'text', required: false, value: bead.parent || '' },
            { id: 'tags', label: 'Tags', type: 'text', required: false, value: tagsValue, description: 'Comma-separated' },
            { id: 'blocked_by', label: 'Blocked by', type: 'text', required: false, value: blockedByValue, description: 'Comma-separated bead IDs' },
            { id: 'blocks', label: 'Blocks', type: 'text', required: false, value: blocksValue, description: 'Comma-separated bead IDs' },
            { id: 'related_to', label: 'Related to', type: 'text', required: false, value: relatedToValue, description: 'Comma-separated bead IDs' },
            { id: 'children', label: 'Children', type: 'text', required: false, value: childrenValue, description: 'Comma-separated bead IDs' },
            { id: 'description', label: 'Description', type: 'textarea', required: false, value: bead.description || '' },
            { id: 'context', label: 'Context (JSON)', type: 'textarea', required: false, value: contextValue }
        ]
    });
    if (!res) return;

    let parsedContext = {};
    if ((res.context || '').trim()) {
        try {
            parsedContext = JSON.parse(res.context);
        } catch (e) {
            showToast('Context must be valid JSON.', 'error');
            return;
        }
    }

    const parseList = (value) => {
        return String(value || '')
            .split(',')
            .map((v) => v.trim())
            .filter((v) => v.length > 0);
    };

    const payload = {
        title: res.title,
        type: res.type,
        status: res.status,
        priority: Number(res.priority),
        project_id: res.project_id,
        assigned_to: res.assigned_to || '',
        parent: res.parent || '',
        tags: parseList(res.tags),
        blocked_by: parseList(res.blocked_by),
        blocks: parseList(res.blocks),
        related_to: parseList(res.related_to),
        children: parseList(res.children),
        description: res.description || '',
        context: parsedContext
    };

    try {
        setBusy(`editBead:${beadId}`, true);
        await saveBeadUpdate(beadId, payload, { successMessage: 'Bead updated' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`editBead:${beadId}`, false);
    }
}

async function redispatchBead(beadId) {
    try {
        const res = await formModal({
            title: 'Request redispatch',
            submitText: 'Request',
            fields: [{ id: 'reason', label: 'Reason (optional)', type: 'textarea', required: false, placeholder: 'Why should this bead be rerun?' }]
        });
        if (!res) return;

        setBusy(`redispatchBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/redispatch`, {
            method: 'POST',
            body: JSON.stringify({ reason: res.reason || '' })
        });

        showToast('Redispatch requested', 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`redispatchBead:${beadId}`, false);
    }
}

async function escalateBead(beadId) {
    try {
        const res = await formModal({
            title: 'Escalate to CEO',
            submitText: 'Escalate',
            fields: [
                { id: 'reason', label: 'Decision needed / reason', type: 'textarea', required: true, placeholder: 'What decision is required?' },
                { id: 'returned_to', label: 'Return to (agent/user id, optional)', type: 'text', required: false, placeholder: 'agent-123 or user-jordan' }
            ]
        });
        if (!res) return;

        setBusy(`escalateBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/escalate`, {
            method: 'POST',
            body: JSON.stringify({ reason: res.reason, returned_to: res.returned_to || '' })
        });

        showToast('Escalated to CEO (decision created)', 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`escalateBead:${beadId}`, false);
    }
}

async function claimDecision(decisionId) {
    try {
        const res = await formModal({
            title: 'Claim decision',
            submitText: 'Claim',
            fields: [
                {
                    id: 'agent_id',
                    label: 'Your user ID (or agent ID)',
                    type: 'text',
                    required: true,
                    placeholder: 'user-jordan or agent-123'
                }
            ]
        });
        if (!res) return;

        setBusy(`claimDecision:${decisionId}`, true);
        await apiCall(`/beads/${decisionId}/claim`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: res.agent_id })
        });

        showToast('Decision claimed', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`claimDecision:${decisionId}`, false);
    }
}

async function makeDecision(decisionId) {
    try {
        const res = await formModal({
            title: 'Make decision',
            submitText: 'Submit decision',
            fields: [
                { id: 'decision', label: 'Decision', type: 'text', required: true, placeholder: 'APPROVE / DENY / ...' },
                { id: 'rationale', label: 'Rationale', type: 'textarea', required: true, placeholder: 'Why?' },
                { id: 'decider_id', label: 'Your user ID', type: 'text', required: true, placeholder: 'user-jordan' }
            ]
        });
        if (!res) return;

        setBusy(`makeDecision:${decisionId}`, true);
        await apiCall(`/decisions/${decisionId}/decide`, {
            method: 'POST',
            body: JSON.stringify({
                decider_id: res.decider_id,
                decision: res.decision,
                rationale: res.rationale
            })
        });

        showToast('Decision submitted', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`makeDecision:${decisionId}`, false);
    }
}

async function editPersona(personaName) {
    const persona = state.personas.find(p => p.name === personaName);
    if (!persona) return;

    try {
        const res = await formModal({
            title: `Edit Persona: ${persona.name}`,
            submitText: 'Save Changes',
            fields: [
                { id: 'character', label: 'Character', type: 'textarea', rows: 6, required: true, value: persona.character || '' },
                { id: 'autonomy_level', label: 'Autonomy Level', type: 'select', required: true, value: persona.autonomy_level || 'moderate',
                  options: [
                    { value: 'minimal', label: 'Minimal - Needs constant guidance' },
                    { value: 'moderate', label: 'Moderate - Some independence' },
                    { value: 'high', label: 'High - Highly autonomous' },
                    { value: 'full', label: 'Full - Completely autonomous' }
                  ]
                },
                { id: 'instructions', label: 'Instructions', type: 'textarea', rows: 10, value: persona.instructions || '' },
                { id: 'tools', label: 'Available Tools (comma-separated)', type: 'text', value: (persona.tools || []).join(', ') }
            ]
        });
        if (!res) return;

        const tools = res.tools ? res.tools.split(',').map(t => t.trim()).filter(t => t) : [];

        await apiCall(`/personas/${personaName}`, {
            method: 'PUT',
            body: JSON.stringify({
                character: res.character,
                autonomy_level: res.autonomy_level,
                instructions: res.instructions,
                tools: tools
            })
        });

        showToast('Persona updated successfully', 'success');
        await loadPersonas();
        render();
    } catch (e) {
        // handled
    }
}

function closePersonaModal() {
    closeModal('persona-modal');
}

// Utilities
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Close modals when clicking outside
window.onclick = function(event) {
    const spawnModal = document.getElementById('spawn-agent-modal');
    const personaModal = document.getElementById('persona-modal');
    const appModal = document.getElementById('app-modal');
    
    if (event.target === spawnModal) {
        closeSpawnAgentModal();
    }
    if (event.target === personaModal) {
        closePersonaModal();
    }
    if (event.target === appModal) {
        closeAppModal();
    }
}

function getFocusableElements(root) {
    const selector = [
        'a[href]',
        'button:not([disabled])',
        'input:not([disabled])',
        'select:not([disabled])',
        'textarea:not([disabled])',
        '[tabindex]:not([tabindex="-1"])'
    ].join(',');

    return Array.from(root.querySelectorAll(selector)).filter(el => {
        // Skip elements that are hidden via display:none.
        return el.offsetParent !== null || el === document.activeElement;
    });
}

function openModal(modalId, options = {}) {
    const modal = document.getElementById(modalId);
    if (!modal) return;

    modalState.lastFocused = document.activeElement;
    modalState.activeId = modalId;

    modal.style.display = 'block';
    modal.classList.add('show');
    modal.setAttribute('aria-hidden', 'false');
    document.body.style.overflow = 'hidden';

    const initial = options.initialFocusSelector ? modal.querySelector(options.initialFocusSelector) : null;
    (initial || modal).focus();
}

function closeModal(modalId) {
    const modal = document.getElementById(modalId);
    if (!modal) return;
    modal.style.display = 'none';
    modal.classList.remove('show');
    modal.setAttribute('aria-hidden', 'true');
    document.body.style.overflow = '';

    modalState.activeId = null;

    if (modalState.lastFocused && typeof modalState.lastFocused.focus === 'function') {
        modalState.lastFocused.focus();
    }
    modalState.lastFocused = null;
}

document.addEventListener('keydown', (event) => {
    if (!modalState.activeId) return;

    const modal = document.getElementById(modalState.activeId);
    if (!modal) return;

    if (event.key === 'Escape') {
        event.preventDefault();
        closeModal(modalState.activeId);
        return;
    }

    if (event.key !== 'Tab') return;

    const focusables = getFocusableElements(modal);
    if (focusables.length === 0) {
        event.preventDefault();
        return;
    }

    const first = focusables[0];
    const last = focusables[focusables.length - 1];

    if (event.shiftKey) {
        if (document.activeElement === first || document.activeElement === modal) {
            event.preventDefault();
            last.focus();
        }
    } else {
        if (document.activeElement === last) {
            event.preventDefault();
            first.focus();
        }
    }
});

function closeAppModal() {
    if (typeof stopConversationAutoRefresh === 'function') stopConversationAutoRefresh();
    // Reset modal width if it was changed
    const modalContent = document.querySelector('#app-modal .modal-content');
    if (modalContent) modalContent.style.maxWidth = '';
    closeModal('app-modal');
}

function openAppModal({ title, bodyHtml, actions = [] }) {
    const titleEl = document.getElementById('app-modal-title');
    const bodyEl = document.getElementById('app-modal-body');
    const actionsEl = document.getElementById('app-modal-actions');
    if (!titleEl || !bodyEl || !actionsEl) return;

    titleEl.textContent = title || 'Dialog';
    bodyEl.innerHTML = bodyHtml || '';
    actionsEl.innerHTML = '';

    for (const a of actions) {
        const btn = document.createElement('button');
        btn.type = 'button';
        if (a.variant) btn.className = a.variant;
        btn.textContent = a.label;
        btn.addEventListener('click', a.onClick);
        actionsEl.appendChild(btn);
    }

    openModal('app-modal');
}

function confirmModal({ title, body, confirmText = 'Confirm', cancelText = 'Cancel', danger = false }) {
    return new Promise((resolve) => {
        openAppModal({
            title,
            bodyHtml: `<p>${escapeHtml(body || '')}</p>`,
            actions: [
                {
                    label: cancelText,
                    variant: 'secondary',
                    onClick: () => {
                        closeAppModal();
                        resolve(false);
                    }
                },
                {
                    label: confirmText,
                    variant: danger ? 'danger' : '',
                    onClick: () => {
                        closeAppModal();
                        resolve(true);
                    }
                }
            ]
        });
    });
}

function formModal({ title, submitText = 'Submit', cancelText = 'Cancel', fields = [] }) {
    return new Promise((resolve) => {
        const formId = `modal-form-${Math.random().toString(16).slice(2)}`;
        const bodyHtml = `
            <form id="${formId}">
                ${fields
                    .map((f) => {
                        const id = `field-${formId}-${f.id}`;
                        const required = f.required ? 'required' : '';
                        const placeholder = f.placeholder ? `placeholder="${escapeHtml(f.placeholder)}"` : '';
                        const readOnly = f.readonly ? 'readonly' : '';
                        const disabled = f.disabled ? 'disabled' : '';
                        const value = f.value !== undefined && f.value !== null ? String(f.value) : '';
                        const description = f.description ? `<div class="small" style="color: var(--text-muted); margin-top: 0.25rem;">${escapeHtml(f.description)}</div>` : '';
                        
                        if (f.type === 'textarea') {
                            return `
                                <label for="${id}">${escapeHtml(f.label)}</label>
                                <textarea id="${id}" name="${escapeHtml(f.id)}" ${required} ${placeholder} ${readOnly} ${disabled}>${escapeHtml(value)}</textarea>
                                ${description}
                            `;
                        }
                        if (f.type === 'select') {
                            const options = Array.isArray(f.options) ? f.options : [];
                            return `
                                <label for="${id}">${escapeHtml(f.label)}</label>
                                <select id="${id}" name="${escapeHtml(f.id)}" ${required} ${disabled}>
                                    ${options
                                        .map((opt) => {
                                            const optValue = String(opt.value ?? '');
                                            const selected = optValue === value ? 'selected' : '';
                                            return `<option value="${escapeHtml(optValue)}" ${selected}>${escapeHtml(opt.label ?? optValue)}</option>`;
                                        })
                                        .join('')}
                                </select>
                                ${description}
                            `;
                        }
                        if (f.type === 'checkbox') {
                            const checked = (value === 'true' || value === true || value === '1') ? 'checked' : '';
                            return `
                                <div style="display: flex; align-items: center; gap: 0.5rem;">
                                    <input type="checkbox" id="${id}" name="${escapeHtml(f.id)}" value="true" ${checked} ${disabled}>
                                    <label for="${id}" style="margin: 0;">${escapeHtml(f.label)}</label>
                                </div>
                                ${description}
                            `;
                        }
                        // Default: text, password, number, etc.
                        const inputType = f.type || 'text';
                        return `
                            <label for="${id}">${escapeHtml(f.label)}</label>
                            <input type="${escapeHtml(inputType)}" id="${id}" name="${escapeHtml(f.id)}" ${required} ${placeholder} ${readOnly} ${disabled} value="${escapeHtml(value)}">
                            ${description}
                        `;
                    })
                    .join('')}
            </form>
        `;

        openAppModal({
            title,
            bodyHtml,
            actions: [
                {
                    label: cancelText,
                    variant: 'secondary',
                    onClick: () => {
                        closeAppModal();
                        resolve(null);
                    }
                },
                {
                    label: submitText,
                    onClick: () => {
                        const form = document.getElementById(formId);
                        if (!form) return;
                        if (!form.reportValidity()) return;
                        const data = new FormData(form);
                        const out = {};
                        for (const [k, v] of data.entries()) out[k] = String(v);
                        closeAppModal();
                        resolve(out);
                    }
                }
            ]
        });

        // focus first field
        window.setTimeout(() => {
            const form = document.getElementById(formId);
            const first = form?.querySelector('input, textarea, select');
            if (first) first.focus();
        }, 0);
    });
}

// ============================================================================
// User Management Functions
// ============================================================================

function renderUsers() {
    const container = document.getElementById('users-list');
    if (!container) return;

    const users = state.users || [];
    const apiKeys = state.apiKeys || [];

    if (users.length === 0) {
        container.innerHTML = renderEmptyState(
            'No Users',
            'User management requires admin access. Login as admin to view users.',
            ''
        );
        return;
    }

    container.innerHTML = `
        <table class="data-table" style="margin-top: 1rem;">
            <thead>
                <tr>
                    <th>Username</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th>Updated</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${users.map(user => `
                    <tr>
                        <td><strong>${escapeHtml(user.username)}</strong></td>
                        <td>${escapeHtml(user.email || '-')}</td>
                        <td><span class="badge badge-${getRoleBadgeClass(user.role)}">${escapeHtml(user.role)}</span></td>
                        <td>${user.is_active ? '<span style="color: var(--success-color);">Active</span>' : '<span style="color: var(--text-muted);">Inactive</span>'}</td>
                        <td class="small">${formatDate(user.created_at)}</td>
                        <td class="small">${formatDate(user.updated_at)}</td>
                        <td>
                            <div class="action-buttons">
                                <button type="button" class="btn-icon" onclick="showEditUserModal('${escapeHtml(user.id || user.username)}')" title="Edit user">✏️</button>
                                <button type="button" class="btn-icon btn-danger" onclick="confirmDeleteUser('${escapeHtml(user.id || user.username)}')" title="Delete user">🗑️</button>
                            </div>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    // Render API keys
    const apikeysContainer = document.getElementById('apikeys-list');
    if (!apikeysContainer) return;

    if (apiKeys.length === 0) {
        apikeysContainer.innerHTML = `<p class="small" style="color: var(--text-muted); margin-top: 1rem;">No API keys. Generate one to access the API programmatically.</p>`;
        return;
    }

    apikeysContainer.innerHTML = `
        <table class="data-table" style="margin-top: 1rem;">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Key Prefix</th>
                    <th>Permissions</th>
                    <th>Status</th>
                    <th>Expires</th>
                    <th>Last Used</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${apiKeys.map(key => {
                    const isExpired = key.expires_at && new Date(key.expires_at) < new Date();
                    return `
                        <tr>
                            <td><strong>${escapeHtml(key.name)}</strong></td>
                            <td><code>${escapeHtml(key.key_prefix)}...</code></td>
                            <td class="small">${(key.permissions || []).join(', ')}</td>
                            <td>${key.is_active && !isExpired ? '<span style="color: var(--success-color);">Active</span>' : '<span style="color: var(--error-color);">Inactive</span>'}</td>
                            <td class="small">${key.expires_at ? formatDate(key.expires_at) : 'Never'}</td>
                            <td class="small">${key.last_used ? formatDate(key.last_used) : 'Never'}</td>
                            <td><button type="button" class="secondary small" onclick="revokeAPIKey('${escapeHtml(key.id)}')">Revoke</button></td>
                        </tr>
                    `;
                }).join('')}
            </tbody>
        </table>
    `;
}

function getRoleBadgeClass(role) {
    const map = {
        admin: 'error',
        user: 'info',
        viewer: 'warning',
        service: 'neutral'
    };
    return map[role] || 'neutral';
}

function renderDiagrams() {
    // Update project filter dropdown
    const projectFilter = document.getElementById('diagram-project-filter');
    if (projectFilter && state.projects) {
        const currentValue = projectFilter.value;
        projectFilter.innerHTML = '<option value="all">All Projects</option>' +
            state.projects.map(p =>
                `<option value="${escapeHtml(p.id)}">${escapeHtml(p.name || p.id)}</option>`
            ).join('');
        // Restore previous selection if it still exists
        if (currentValue && Array.from(projectFilter.options).some(opt => opt.value === currentValue)) {
            projectFilter.value = currentValue;
        }
    }

    // Update legend visibility based on current diagram type
    if (typeof diagramState !== 'undefined') {
        const legendHierarchy = document.getElementById('legend-hierarchy');
        const legendMotivation = document.getElementById('legend-motivation');
        const legendMessage = document.getElementById('legend-message');

        if (legendHierarchy) legendHierarchy.style.display = diagramState.currentType === 'hierarchy' ? 'flex' : 'none';
        if (legendMotivation) legendMotivation.style.display = diagramState.currentType === 'motivation' ? 'flex' : 'none';
        if (legendMessage) legendMessage.style.display = diagramState.currentType === 'message' ? 'flex' : 'none';
    }

    // If diagrams view is active, update diagram data
    if (uiState.view.active === 'diagrams' && typeof updateDiagramData === 'function') {
        updateDiagramData();
    }
}

async function handleCreateUser() {
    const username = document.getElementById('user-username')?.value;
    const email = document.getElementById('user-email')?.value;
    const password = document.getElementById('user-password')?.value;
    const role = document.getElementById('user-role')?.value;

    if (!username || !password || !role) {
        showToast('Username, password, and role are required', 'error');
        return;
    }

    try {
        await apiCall('/auth/users', {
            method: 'POST',
            body: JSON.stringify({
                username,
                email,
                password,
                role
            })
        });

        showToast(`User ${username} created successfully`, 'success');
        document.getElementById('create-user-form').style.display = 'none';
        document.getElementById('user-form').reset();
        await loadUsers();
        render();
    } catch (e) {
        showToast(`Failed to create user: ${e.message}`, 'error');
    }
}

async function handleCreateAPIKey() {
    const name = document.getElementById('apikey-name')?.value;
    const permissionsEl = document.getElementById('apikey-permissions');
    const expiresIn = parseInt(document.getElementById('apikey-expires')?.value || '0');

    if (!name) {
        showToast('API key name is required', 'error');
        return;
    }

    const permissions = Array.from(permissionsEl?.selectedOptions || []).map(opt => opt.value);

    try {
        const response = await apiCall('/auth/api-keys', {
            method: 'POST',
            body: JSON.stringify({
                name,
                permissions,
                expires_in: expiresIn
            })
        });

        // Hide form, show key display
        document.getElementById('create-apikey-form').style.display = 'none';
        document.getElementById('apikey-form').reset();
        
        const display = document.getElementById('apikey-display');
        const valueEl = document.getElementById('apikey-value');
        if (display && valueEl && response.key) {
            valueEl.textContent = response.key;
            display.style.display = 'block';
        }

        showToast('API key created successfully', 'success');
        await loadAPIKeys();
        render();
    } catch (e) {
        showToast(`Failed to create API key: ${e.message}`, 'error');
    }
}

async function revokeAPIKey(keyId) {
    if (!confirm('Are you sure you want to revoke this API key? This action cannot be undone.')) {
        return;
    }

    try {
        await apiCall(`/auth/api-keys/${keyId}`, {
            method: 'DELETE'
        });

        showToast('API key revoked', 'success');
        await loadAPIKeys();
        render();
    } catch (e) {
        showToast(`Failed to revoke API key: ${e.message}`, 'error');
    }
}

// Analytics Dashboard Functions
async function loadAnalytics() {
    const timeRange = document.getElementById('analytics-time-range').value;
    let startTime, endTime;
    
    const now = new Date();
    endTime = now.toISOString();
    
    switch (timeRange) {
        case '1h':
            startTime = new Date(now - 60 * 60 * 1000).toISOString();
            break;
        case '24h':
            startTime = new Date(now - 24 * 60 * 60 * 1000).toISOString();
            break;
        case '7d':
            startTime = new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case '30d':
            startTime = new Date(now - 30 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case 'custom':
            startTime = document.getElementById('analytics-start-time').value;
            endTime = document.getElementById('analytics-end-time').value;
            if (!startTime || !endTime) {
                showToast('Please select start and end times', 'warning');
                return;
            }
            break;
    }
    
    try {
        const params = new URLSearchParams();
        if (startTime) params.append('start_time', startTime);
        if (endTime) params.append('end_time', endTime);
        
        const response = await fetch(`/api/v1/analytics/stats?${params}`, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) throw new Error('Failed to load analytics');
        
        const stats = await response.json();
        renderAnalytics(stats);
        
        const batchingParams = new URLSearchParams(params.toString());
        batchingParams.append('auto_batch', 'true');
        await loadBatchingRecommendations(batchingParams);
    } catch (error) {
        console.error('Error loading analytics:', error);
        showToast('Failed to load analytics: ' + error.message, 'error');
    }
}

function cleanAgentKeys(data) {
    const out = {};
    Object.keys(data).forEach(function (k) {
        const clean = k.replace(/^agent:/, '');
        out[clean] = data[k];
    });
    return out;
}

function renderAnalytics(stats) {
    // Animated summary counters (D3)
    const reqEl = document.getElementById('analytics-total-requests');
    const tokEl = document.getElementById('analytics-total-tokens');
    const costEl = document.getElementById('analytics-total-cost');
    const latEl = document.getElementById('analytics-avg-latency');
    if (typeof LoomCharts !== 'undefined') {
        if (reqEl) LoomCharts.animateCounter(reqEl, stats.total_requests);
        if (tokEl) LoomCharts.animateCounter(tokEl, stats.total_tokens);
        if (costEl) LoomCharts.animateCounter(costEl, stats.total_cost_usd, { prefix: '$', decimals: 2 });
        if (latEl) LoomCharts.animateCounter(latEl, Math.round(stats.avg_latency_ms / 1000), { suffix: 's' });

        // Error rate gauge
        const gaugeEl = document.getElementById('analytics-error-gauge');
        if (gaugeEl) LoomCharts.gauge(gaugeEl, stats.error_rate || 0, { label: (stats.error_rate * 100).toFixed(1) + '%' });

        // Agent workload bars
        const agentReqBar = document.getElementById('d3-bar-requests-agent');
        if (agentReqBar) {
            const cleanAgentReqs = cleanAgentKeys(stats.requests_by_user || {});
            LoomCharts.barChart(agentReqBar, cleanAgentReqs, { labelWidth: 150 });
        }

        const agentTokBar = document.getElementById('d3-bar-tokens-agent');
        if (agentTokBar) {
            const cleanAgentToks = cleanAgentKeys(stats.tokens_by_user || stats.requests_by_user || {});
            LoomCharts.barChart(agentTokBar, cleanAgentToks, { labelWidth: 150 });
        }
    } else {
        // Fallback: plain text
        if (reqEl) reqEl.textContent = stats.total_requests.toLocaleString();
        if (tokEl) tokEl.textContent = LoomCharts ? LoomCharts.shortNum(stats.total_tokens) : stats.total_tokens.toLocaleString();
        if (costEl) costEl.textContent = '$' + stats.total_cost_usd.toFixed(2);
        if (latEl) latEl.textContent = Math.round(stats.avg_latency_ms);
    }

    // Cost charts (legacy bar chart as fallback until costs are configured)
    renderBarChart('chart-cost-by-user', stats.cost_by_user || {}, '$');

    // Render detailed table
    renderAnalyticsTable(stats);
}

async function loadBatchingRecommendations(params) {
    const tbody = document.getElementById('analytics-batching-table');
    try {
        const query = params ? params.toString() : '';
        const response = await fetch(`/api/v1/analytics/batching?${query}`, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) throw new Error('Failed to load batching recommendations');
        
        const recommendations = await response.json();
        renderBatchingRecommendations(recommendations);
    } catch (error) {
        console.error('Error loading batching recommendations:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">Unable to load batching recommendations</td></tr>';
        }
        showToast('Failed to load batching recommendations: ' + error.message, 'error');
    }
}

function renderBatchingRecommendations(data) {
    const summary = data?.summary || {};
    const requests = summary.batchable_requests || 0;
    const batches = summary.recommended_batches || 0;
    const savings = summary.estimated_cost_savings_usd || 0;
    const avgBatchSize = summary.average_batch_size || 0;

    const requestsEl = document.getElementById('analytics-batching-requests');
    const batchesEl = document.getElementById('analytics-batching-batches');
    const savingsEl = document.getElementById('analytics-batching-savings');
    const avgBatchEl = document.getElementById('analytics-batching-batch-size');

    if (requestsEl) requestsEl.textContent = requests.toLocaleString();
    if (batchesEl) batchesEl.textContent = batches.toLocaleString();
    if (savingsEl) savingsEl.textContent = '$' + savings.toFixed(4);
    if (avgBatchEl) avgBatchEl.textContent = avgBatchSize ? avgBatchSize.toFixed(1) : '-';

    const tbody = document.getElementById('analytics-batching-table');
    if (!tbody) return;

    const recommendations = data?.recommendations || [];
    if (recommendations.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No batchable requests detected</td></tr>';
        return;
    }

    tbody.innerHTML = recommendations.slice(0, 10).map((rec) => {
        const endpoint = `${rec.method || ''} ${rec.path || ''}`.trim();
        const provider = rec.provider_id || 'unknown';
        const window = formatBatchingWindow(rec.time_window_start, rec.time_window_end);
        const savingsValue = rec.estimated_cost_savings_usd || 0;
        return `
            <tr>
                <td>${escapeHtml(endpoint || '-')}
                    <div class="small" style="color: var(--text-muted);">${escapeHtml(rec.model_name || 'model n/a')}</div>
                </td>
                <td>${escapeHtml(provider)}</td>
                <td>${(rec.request_count || 0).toLocaleString()}</td>
                <td>${rec.batch_size || '-'}</td>
                <td>$${savingsValue.toFixed(4)}</td>
                <td class="small">${escapeHtml(window)}</td>
            </tr>
        `;
    }).join('');
}

function formatBatchingWindow(start, end) {
    if (!start || !end) return '-';
    const startDate = new Date(start);
    const endDate = new Date(end);
    if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
        return '-';
    }
    const startTime = startDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    const endTime = endDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return `${startTime} - ${endTime}`;
}

function renderBarChart(elementId, data, prefix = '') {
    const container = document.getElementById(elementId);
    if (!container) return;
    
    const entries = Object.entries(data).sort((a, b) => b[1] - a[1]);
    
    if (entries.length === 0) {
        container.innerHTML = '<p class="small" style="text-align: center; padding: 2rem;">No data available</p>';
        return;
    }
    
    const maxValue = Math.max(...entries.map(([, value]) => value));
    
    container.innerHTML = entries.map(([key, value]) => {
        const percentage = maxValue > 0 ? (value / maxValue) * 100 : 0;
        const displayValue = prefix ? `${prefix}${value.toFixed(2)}` : value;
        
        return `
            <div class="chart-bar">
                <div class="chart-bar-label" title="${key}">${truncateText(key, 15)}</div>
                <div class="chart-bar-container">
                    <div class="chart-bar-fill" style="width: ${percentage}%"></div>
                </div>
                <div class="chart-bar-value">${displayValue}</div>
            </div>
        `;
    }).join('');
}

function renderAnalyticsTable(stats) {
    const tbody = document.getElementById('analytics-details-table');
    if (!tbody) return;
    
    const rows = [];
    
    // Provider rows
    const providers = Object.keys(stats.cost_by_provider || {});
    providers.forEach(provider => {
        rows.push({
            type: 'Provider',
            name: provider,
            requests: stats.requests_by_provider?.[provider] || 0,
            cost: stats.cost_by_provider?.[provider] || 0
        });
    });
    
    // User rows
    const users = Object.keys(stats.cost_by_user || {});
    users.forEach(user => {
        rows.push({
            type: 'User',
            name: user,
            requests: stats.requests_by_user?.[user] || 0,
            cost: stats.cost_by_user?.[user] || 0
        });
    });
    
    if (rows.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No data available</td></tr>';
        return;
    }
    
    tbody.innerHTML = rows.map(row => {
        const avgLatency = row.requests > 0 ? (stats.avg_latency_ms || 0) : 0;
        const tokens = Math.round((stats.total_tokens || 0) * (row.requests / (stats.total_requests || 1)));
        
        return `
            <tr>
                <td>${row.type}</td>
                <td><strong>${row.name}</strong></td>
                <td>${row.requests.toLocaleString()}</td>
                <td>${tokens.toLocaleString()}</td>
                <td>$${row.cost.toFixed(2)}</td>
                <td>${Math.round(avgLatency)}</td>
            </tr>
        `;
    }).join('');
}

function truncateText(text, maxLength) {
    if (!text) return '';
    return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
}

// Event listeners for analytics
document.getElementById('analytics-time-range')?.addEventListener('change', function() {
    const customRange = document.getElementById('analytics-custom-range');
    if (this.value === 'custom') {
        customRange.style.display = 'flex';
        customRange.style.gap = '0.5rem';
    } else {
        customRange.style.display = 'none';
        loadAnalytics();
    }
});

document.getElementById('refresh-analytics-btn')?.addEventListener('click', loadAnalytics);

// Load analytics when tab is shown
document.querySelectorAll('.view-tab').forEach(tab => {
    const originalClick = tab.onclick;
    tab.onclick = function() {
        if (originalClick) originalClick.call(this);
        if (this.dataset.target === 'analytics' && state.token) {
            setTimeout(loadAnalytics, 100);
        }
    };
});

// Export analytics data
async function exportAnalytics(type, format) {
    const timeRange = document.getElementById('analytics-time-range').value;
    let startTime, endTime;
    
    const now = new Date();
    endTime = now.toISOString();
    
    switch (timeRange) {
        case '1h':
            startTime = new Date(now - 60 * 60 * 1000).toISOString();
            break;
        case '24h':
            startTime = new Date(now - 24 * 60 * 60 * 1000).toISOString();
            break;
        case '7d':
            startTime = new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case '30d':
            startTime = new Date(now - 30 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case 'custom':
            startTime = document.getElementById('analytics-start-time').value;
            endTime = document.getElementById('analytics-end-time').value;
            break;
    }
    
    try {
        const params = new URLSearchParams();
        if (startTime) params.append('start_time', startTime);
        if (endTime) params.append('end_time', endTime);
        if (format) params.append('format', format);
        
        const endpoint = type === 'logs' ? '/api/v1/analytics/export' : '/api/v1/analytics/export-stats';
        const url = `${endpoint}?${params}`;
        
        // Create a temporary link to trigger download
        const a = document.createElement('a');
        a.href = url;
        a.setAttribute('download', '');
        a.style.display = 'none';
        
        // Add authorization header via fetch and create blob URL
        const response = await fetch(url, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Export failed: ' + response.statusText);
        }
        
        const blob = await response.blob();
        const blobUrl = URL.createObjectURL(blob);
        a.href = blobUrl;
        
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        
        // Clean up blob URL after a short delay
        setTimeout(() => URL.revokeObjectURL(blobUrl), 100);
        
        showToast('Export started. Your download should begin shortly.', 'success');
    } catch (error) {
        console.error('Error exporting analytics:', error);
        showToast('Failed to export: ' + error.message, 'error');
    }
}

// Event listeners for export buttons
document.getElementById('export-stats-json-btn')?.addEventListener('click', () => {
    exportAnalytics('stats', 'json');
});

document.getElementById('export-stats-csv-btn')?.addEventListener('click', () => {
    exportAnalytics('stats', 'csv');
});

document.getElementById('export-logs-csv-btn')?.addEventListener('click', () => {
    exportAnalytics('logs', 'csv');
});

// Change Velocity Widget Functions
async function loadChangeVelocity(projectID) {
    if (!projectID) {
        // Default to first project or show message
        const projects = await loadProjects();
        if (projects && projects.length > 0) {
            projectID = projects[0].id;
        } else {
            console.warn('No projects available for change velocity');
            return;
        }
    }

    try {
        const response = await fetch(`/api/v1/analytics/change-velocity?project_id=${projectID}&time_window=24h`, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });

        if (!response.ok) throw new Error('Failed to load change velocity');

        const metrics = await response.json();
        renderChangeVelocityWidget(metrics);
    } catch (error) {
        console.error('Error loading change velocity:', error);
        // Don't show toast - this is a background widget update
    }
}

function renderChangeVelocityWidget(metrics) {
    const container = document.getElementById('change-velocity-widget');
    if (!container) return;

    const uncommittedCount = metrics.uncommitted_files ? metrics.uncommitted_files.length : 0;
    const uncommittedClass = uncommittedCount > 0 ? 'text-red-600 font-bold' : 'text-green-600';

    const html = `
        <div class="bg-white rounded-lg shadow p-4">
            <h3 class="text-lg font-semibold mb-3">Change Velocity (24h)</h3>
            <div class="grid grid-cols-2 gap-4">
                <div>
                    <div class="text-sm text-gray-600">Commits/Day</div>
                    <div class="text-2xl font-bold">${metrics.change_velocity.toFixed(1)}</div>
                </div>
                <div>
                    <div class="text-sm text-gray-600">Uncommitted Files</div>
                    <div class="text-2xl font-bold ${uncommittedClass}">${uncommittedCount}</div>
                </div>
            </div>
            <div class="mt-4">
                <div class="text-sm font-semibold mb-2">Development Funnel</div>
                <div class="space-y-1 text-sm">
                    <div class="flex justify-between">
                        <span>Files Modified:</span>
                        <span class="font-mono">${metrics.files_modified}</span>
                    </div>
                    <div class="flex justify-between">
                        <span>Builds Passed:</span>
                        <span class="font-mono">${metrics.builds_succeeded}/${metrics.builds_attempted}</span>
                    </div>
                    <div class="flex justify-between">
                        <span>Tests Passed:</span>
                        <span class="font-mono">${metrics.tests_passed}/${metrics.tests_attempted}</span>
                    </div>
                    <div class="flex justify-between">
                        <span>Commits:</span>
                        <span class="font-mono">${metrics.commits_succeeded}/${metrics.commits_attempted}</span>
                    </div>
                    <div class="flex justify-between">
                        <span>Pushes:</span>
                        <span class="font-mono">${metrics.pushes_succeeded}/${metrics.pushes_attempted}</span>
                    </div>
                </div>
            </div>
            ${uncommittedCount > 0 ? `
                <div class="mt-3 p-2 bg-red-50 border border-red-200 rounded text-xs">
                    <div class="font-semibold text-red-800">⚠️ Uncommitted Changes:</div>
                    <div class="text-red-700 mt-1 max-h-20 overflow-y-auto">
                        ${metrics.uncommitted_files.slice(0, 5).join('<br>')}
                        ${uncommittedCount > 5 ? `<br><em>...and ${uncommittedCount - 5} more</em>` : ''}
                    </div>
                </div>
            ` : ''}
        </div>
    `;

    container.innerHTML = html;
}

// Load change velocity widget on dashboard view
document.querySelectorAll('.view-tab').forEach(tab => {
    const originalClick = tab.onclick;
    tab.onclick = function() {
        if (originalClick) originalClick.call(this);
        if (this.dataset.target === 'home' && state.token && state.currentProject) {
            setTimeout(() => loadChangeVelocity(state.currentProject), 100);
        }
    };
});

// =====================
// Motivations Dashboard
// =====================

let motivationsState = {
    motivations: [],
    roles: [],
    history: [],
    idleState: null
};

// Expose motivationsState to window for diagrams
window.motivationsState = motivationsState;

async function loadMotivations() {
    try {
        // Load all motivations data in parallel
        const [motivationsRes, rolesRes, historyRes, idleRes] = await Promise.all([
            fetch(`${API_BASE}/motivations`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/roles`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/history?limit=50`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/idle`, { headers: getAuthHeaders() })
        ]);

        if (motivationsRes.ok) {
            const data = await motivationsRes.json();
            // API returns {count, motivations}, extract the array
            motivationsState.motivations = data.motivations || data || [];
        }
        if (rolesRes.ok) {
            const rolesData = await rolesRes.json();
            // API returns {motivations: {role: [...]}} - transform to {roles: [{role, motivations}]}
            if (rolesData.motivations && typeof rolesData.motivations === 'object') {
                motivationsState.roles = {
                    roles: Object.entries(rolesData.motivations).map(([role, motivations]) => ({
                        role,
                        motivations: motivations || []
                    }))
                };
            } else {
                motivationsState.roles = rolesData;
            }
            populateMotivationRoleFilter();
        }
        if (historyRes.ok) {
            const historyData = await historyRes.json();
            motivationsState.history = historyData.history || [];
        }
        if (idleRes.ok) {
            motivationsState.idleState = await idleRes.json();
        }

        renderMotivationsDashboard();
    } catch (error) {
        console.error('Error loading motivations:', error); alert('Failed to load motivations. Please check your network connection and try again.');
        showToast('Failed to load motivations: ' + error.message, 'error');
    }
}

function populateMotivationRoleFilter() {
    const select = document.getElementById('motivation-role-filter');
    if (!select || !motivationsState.roles) return;

// Load Active Meetings
async function loadActiveMeetings() {
    try {
        state.activeMeetings = await apiCall('/api/v1/meetings/active');
    } catch (err) {
        console.error('[Loom] Failed to load active meetings:', err);
        state.activeMeetings = [];
    }
}

// Load Status Board Feed
async function loadStatusBoardFeed() {
    try {
        state.statusBoardFeed = await apiCall('/api/v1/statusboard');
    } catch (err) {
        console.error('[Loom] Failed to load status board feed:', err);
        state.statusBoardFeed = [];
    }
}

// Load Org Health
async function loadOrgHealth() {
    try {
        state.orgHealth = await apiCall('/api/v1/org-chart/live');
    } catch (err) {
        console.error('[Loom] Failed to load org health:', err);
        state.orgHealth = {};
    }
}

// Load Review Summary
async function loadReviewSummary() {
    try {
        state.reviewSummary = await apiCall('/api/v1/reviews');
    } catch (err) {
        console.error('[Loom] Failed to load review summary:', err);
        state.reviewSummary = {};
    }
}

// Load Escalation Queue
async function loadEscalationQueue() {
    try {
        // Filter decisions that require human input
        const allDecisions = state.decisions || [];
        state.escalationQueue = allDecisions.filter(d => d.requires_human === true);
    } catch (err) {
        console.error('[Loom] Failed to load escalation queue:', err);
        state.escalationQueue = [];
    }
}

// Load Escalation Queue
async function loadEscalationQueue() {
    try {
        // Filter decisions that require human input
        const allDecisions = state.decisions || [];
        state.escalationQueue = allDecisions.filter(d => d.requires_human === true);
    } catch (err) {
        console.error('[Loom] Failed to load escalation queue:', err);
        state.escalationQueue = [];
    }
}
    
    // Keep the "All Roles" option
    select.innerHTML = '<option value="">All Roles</option>';
    
    // Add role options
    const roles = motivationsState.roles.roles || [];
    roles.forEach(role => {
        const option = document.createElement('option');
        option.value = role.role;
        option.textContent = role.role.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        select.appendChild(option);
    });
}

function renderMotivationsDashboard() {
    renderMotivationStats();
    renderMotivationsByRole();
    renderMotivationsTable();
    renderMotivationHistory();
}

function renderMotivationStats() {
    // System idle status
    const idleEl = document.getElementById('motivation-system-idle');
    const idleLabelEl = document.getElementById('motivation-idle-label');
    if (idleEl && motivationsState.idleState) {
        const isIdle = motivationsState.idleState.system_idle;
        idleEl.textContent = isIdle ? '💤' : '✅';
        idleEl.style.color = isIdle ? 'var(--warning-color)' : 'var(--success-color)';
        idleLabelEl.textContent = isIdle ? 'System Idle' : 'Active';
    }

    // Total count
    const totalEl = document.getElementById('motivation-total-count');
    if (totalEl) {
        totalEl.textContent = motivationsState.motivations.length || 0;
    }

    // Active count
    const activeEl = document.getElementById('motivation-active-count');
    if (activeEl) {
        const activeCount = motivationsState.motivations.filter(m => m.status === 'active').length;
        activeEl.textContent = activeCount;
    }

    // Recent triggers (last 24h)
    const triggersEl = document.getElementById('motivation-recent-triggers');
    if (triggersEl) {
        const now = Date.now();
        const oneDayAgo = now - 24 * 60 * 60 * 1000;
        const recentCount = (motivationsState.history || []).filter(h => {
            const triggerTime = new Date(h.triggered_at).getTime();
            return triggerTime > oneDayAgo;
        }).length;
        triggersEl.textContent = recentCount;
    }
}

function renderMotivationsByRole() {
    const container = document.getElementById('motivations-by-role');
    if (!container || !motivationsState.roles) return;

    const roles = motivationsState.roles.roles || [];
    if (roles.length === 0) {
        container.innerHTML = '<p>No motivations registered.</p>';
        return;
    }

    container.innerHTML = roles.map(roleData => {
        const motivations = roleData.motivations || [];
        const activeCount = motivations.filter(m => m.status === 'active').length;
        
        return `
            <div class="motivation-role-card">
                <h4>${roleData.role.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}</h4>
                <div class="motivation-role-stats">
                    <span class="stat">${motivations.length} total</span>
                    <span class="stat active">${activeCount} active</span>
                </div>
                <ul class="motivation-list">
                    ${motivations.slice(0, 3).map(m => `
                        <li class="${m.status}">
                            <span class="motivation-type type-${m.type}">${m.type}</span>
                            ${m.name}
                        </li>
                    `).join('')}
                    ${motivations.length > 3 ? `<li class="more">+${motivations.length - 3} more</li>` : ''}
                </ul>
            </div>
        `;
    }).join('');
}

function renderMotivationsTable() {
    const tbody = document.getElementById('motivations-table-body');
    if (!tbody) return;

    // Apply filters
    const typeFilter = document.getElementById('motivation-type-filter')?.value || '';
    const roleFilter = document.getElementById('motivation-role-filter')?.value || '';
    const statusFilter = document.getElementById('motivation-status-filter')?.value || '';

    let filtered = motivationsState.motivations;
    if (typeFilter) {
        filtered = filtered.filter(m => m.type === typeFilter);
    }
    if (roleFilter) {
        filtered = filtered.filter(m => m.agent_role === roleFilter);
    }
    if (statusFilter) {
        filtered = filtered.filter(m => m.status === statusFilter);
    }

    if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" style="text-align: center;">No motivations found.</td></tr>';
        return;
    }

    tbody.innerHTML = filtered.map(m => {
        const cooldownMins = m.cooldown_minutes || Math.round((m.cooldown_period || 0) / 60000000000);
        const statusClass = m.status === 'active' ? 'success' : 'muted';
        const isBuiltIn = m.is_built_in || m.built_in;
        
        return `
            <tr>
                <td>
                    <strong>${escapeHtml(m.name)}</strong>
                    ${isBuiltIn ? '<span class="badge">built-in</span>' : ''}
                </td>
                <td><span class="motivation-type type-${m.type}">${m.type}</span></td>
                <td>${escapeHtml(m.condition)}</td>
                <td>${escapeHtml(m.agent_role || '-')}</td>
                <td>${m.priority || 50}</td>
                <td>${cooldownMins}m</td>
                <td><span class="status-${statusClass}">${m.status}</span></td>
                <td>
                    ${m.status === 'active' 
                        ? `<button class="small secondary" onclick="disableMotivation('${m.id}')">Disable</button>`
                        : `<button class="small primary" onclick="enableMotivation('${m.id}')">Enable</button>`
                    }
                    <button class="small" onclick="triggerMotivation('${m.id}')">Trigger</button>
                </td>
            </tr>
        `;
    }).join('');
}

function renderMotivationHistory() {
    const tbody = document.getElementById('motivation-history-table');
    if (!tbody) return;

    const history = motivationsState.history || [];
    if (history.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align: center;">No trigger history.</td></tr>';
        return;
    }

    tbody.innerHTML = history.slice(0, 20).map(h => {
        const date = new Date(h.triggered_at);
        const timeAgo = formatTimeAgo(date);
        const resultClass = h.result === 'success' ? 'success' : (h.result === 'error' ? 'error' : 'muted');
        
        return `
            <tr>
                <td>${escapeHtml(h.motivation_name || h.motivation_id)}</td>
                <td>${escapeHtml(h.agent_role || '-')}</td>
                <td title="${date.toISOString()}">${timeAgo}</td>
                <td><span class="status-${resultClass}">${h.result || '-'}</span></td>
            </tr>
        `;
    }).join('');
}

function formatTimeAgo(date) {
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    return `${diffDays}d ago`;
}

async function enableMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/enable`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to enable motivation');
        showToast('Motivation enabled', 'success');
        loadMotivations();
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

async function disableMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/disable`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to disable motivation');
        showToast('Motivation disabled', 'success');
        loadMotivations();
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

async function triggerMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/trigger`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to trigger motivation');
        showToast('Motivation triggered manually', 'success');
        // Reload to see the new history entry
        setTimeout(loadMotivations, 1000);
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

// Motivation event listeners
document.getElementById('refresh-motivations-btn')?.addEventListener('click', loadMotivations);
document.getElementById('motivation-type-filter')?.addEventListener('change', renderMotivationsTable);
document.getElementById('motivation-role-filter')?.addEventListener('change', renderMotivationsTable);
document.getElementById('motivation-status-filter')?.addEventListener('change', renderMotivationsTable);

// --- Agent Conversation Viewer ---

let conversationAutoRefreshTimer = null;

function stopConversationAutoRefresh() {
    if (conversationAutoRefreshTimer) {
        clearInterval(conversationAutoRefreshTimer);
        conversationAutoRefreshTimer = null;
    }
}

async function viewAgentConversation(beadId) {
    if (!beadId || beadId === 'undefined' || beadId === 'null') {
        showToast('No bead ID available for this agent', 'error');
        return;
    }
    stopConversationAutoRefresh();

    const bodyHtml = `<div id="conv-container" class="conv-container"><div class="loading">Loading conversation...</div></div>`;
    openAppModal({ title: 'Agent Conversation', bodyHtml });

    // Make the modal wider for conversation view
    const modalContent = document.querySelector('#app-modal .modal-content');
    if (modalContent) modalContent.style.maxWidth = '900px';

    await refreshConversation(beadId);

    // Auto-refresh every 10s while modal is open
    conversationAutoRefreshTimer = setInterval(() => {
        if (modalState.activeId === 'app-modal') {
            refreshConversation(beadId);
        } else {
            stopConversationAutoRefresh();
        }
    }, 10000);
}

async function refreshConversation(beadId) {
    const container = document.getElementById('conv-container');
    if (!container) return;
    if (!beadId || beadId === 'undefined' || beadId === 'null') {
        container.innerHTML = '<div class="empty-state"><p>No bead ID provided.</p></div>';
        return;
    }

    try {
        const session = await apiCall(`/beads/${encodeURIComponent(beadId)}/conversation`, { suppressToast: true, skipAutoFile: true });
        if (!session || !session.messages || session.messages.length === 0) {
            container.innerHTML = '<div class="empty-state"><p>No conversation messages yet.</p></div>';
            return;
        }
        container.innerHTML = renderConversationMessages(session.messages);
        // Scroll to bottom
        container.scrollTop = container.scrollHeight;
    } catch (error) {
        if (container.innerHTML.includes('Loading')) {
            container.innerHTML = `<div class="empty-state"><p>No conversation found for this bead.</p></div>`;
        }
    }
}

function renderConversationMessages(messages) {
    return messages.map(msg => {
        const roleClass = msg.role === 'system' ? 'conv-system' :
                         msg.role === 'user' ? 'conv-user' : 'conv-assistant';
        const roleLabel = msg.role === 'system' ? 'System' :
                         msg.role === 'user' ? 'Feedback' : 'Agent';
        const time = msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString() : '';

        return `
            <div class="conv-message ${roleClass}">
                <div class="conv-message-header">
                    <span class="conv-role">${roleLabel}</span>
                    ${time ? `<span class="conv-time">${time}</span>` : ''}
                </div>
                <div class="conv-message-body">${formatConversationContent(msg.content, msg.role)}</div>
            </div>
        `;
    }).join('');
}

function formatConversationContent(content, role) {
    if (!content) return '';
    let text = escapeHtml(content);

    // Truncate very long system prompts
    if (role === 'system' && text.length > 500) {
        const preview = text.substring(0, 500);
        return `<div class="conv-truncated">${preview.replace(/\n/g, '<br>')}...<br><small>[System prompt truncated]</small></div>`;
    }

    // For assistant messages, try to show JSON actions compactly
    if (role === 'assistant') {
        try {
            const parsed = JSON.parse(content);
            if (parsed.actions && Array.isArray(parsed.actions)) {
                const summary = parsed.actions.map(a => {
                    let detail = a.type;
                    if (a.path) detail += `: ${a.path}`;
                    else if (a.command) detail += `: ${a.command.substring(0, 60)}`;
                    else if (a.bead && a.bead.title) detail += `: ${a.bead.title}`;
                    return `<span class="conv-action-tag">${escapeHtml(detail)}</span>`;
                }).join(' ');
                let html = `<div class="conv-actions-summary">${summary}</div>`;
                if (parsed.thinking) {
                    html = `<div class="conv-thinking">${escapeHtml(parsed.thinking.substring(0, 300))}${parsed.thinking.length > 300 ? '...' : ''}</div>` + html;
                }
                return html;
            }
        } catch (e) { /* not JSON, render as text */ }
    }

    // Basic markdown formatting
    text = text.replace(/```([^`]+)```/g, '<pre class="conv-code">$1</pre>');
    text = text.replace(/`([^`]+)`/g, '<code>$1</code>');
    text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    text = text.replace(/## ([^\n]+)/g, '<strong>$1</strong>');
    text = text.replace(/\n/g, '<br>');
    return text;
}

// Toggle observability menu dropdown
function toggleObservabilityMenu() {
    const dropdown = document.getElementById('observability-dropdown');
    if (dropdown.style.display === 'none') {
        dropdown.style.display = 'block';
    } else {
        dropdown.style.display = 'none';
    }
}

// Close observability menu when clicking outside
document.addEventListener('click', function(event) {
    const menu = document.querySelector('.observability-menu');
    const dropdown = document.getElementById('observability-dropdown');
    if (menu && dropdown && !menu.contains(event.target)) {
        dropdown.style.display = 'none';
    }
});

// TEST BUG: Intentional error for self-healing workflow demonstration
// This should be detected, auto-filed, investigated, and fixed by Loom agents
// FIXED: Commented out to prevent UI errors on every page load
// testSelfHealingWorkflow();

// Render Active Meetings
function renderActiveMeetings() {
    const container = document.getElementById('active-meetings-container');
    if (!container) return;
    
    const meetings = state.activeMeetings || [];
    if (meetings.length === 0) {
        container.innerHTML = renderEmptyState('No active meetings', 'Meetings will appear here.');
        return;
    }
    
    container.innerHTML = `
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 1rem;">
            ${meetings.map(m => `
                <div style="padding: 1rem; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary);">
                    <h4>${escapeHtml(m.title || 'Untitled Meeting')}</h4>
                    <p class="small" style="margin: 0.5rem 0; color: var(--text-muted);">Started: ${new Date(m.start_time).toLocaleString()}</p>
                    <p class="small" style="margin: 0.5rem 0;"><strong>Participants:</strong> ${(m.participants || []).join(', ')}</p>
                    ${m.transcript ? `<button type="button" class="secondary" onclick="alert('Transcript: ' + ${JSON.stringify(m.transcript)})" style="margin-top: 0.5rem;">View Transcript</button>` : ''}
                </div>
            `).join('')}
        </div>
    `;
}

// Render Status Board Feed
function renderStatusBoardFeed() {
    const container = document.getElementById('status-board-feed-container');
    if (!container) return;
    
    const feed = state.statusBoardFeed || [];
    if (feed.length === 0) {
        container.innerHTML = renderEmptyState('No status updates', 'Status board entries will appear here.');
        return;
    }
    
    container.innerHTML = `
        <div style="display: flex; flex-direction: column; gap: 1rem;">
            ${feed.slice(0, 10).map(entry => `
                <div style="padding: 1rem; border-left: 3px solid #0ea5e9; background: var(--bg-secondary); border-radius: 4px;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                        <strong>${escapeHtml(entry.author_display_name || 'Unknown')}</strong>
                        <span class="small" style="color: var(--text-muted);">${new Date(entry.timestamp).toLocaleString()}</span>
                    </div>
                    <p style="margin: 0; white-space: pre-wrap;">${escapeHtml(entry.content || '')}</p>
                </div>
            `).join('')}
        </div>
    `;
}

// Render Org Health
function renderOrgHealth() {
    const container = document.getElementById('org-health-container');
    if (!container) return;
    
    const health = state.orgHealth || {};
    const working = health.working || [];
    const idle = health.idle || [];
    const blocked = health.blocked || [];
    const vacant = health.vacant_positions || [];
    
    container.innerHTML = `
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 1rem;">
            <div style="padding: 1rem; border: 1px solid #16a34a; border-radius: 4px; background: rgba(22, 163, 74, 0.05);">
                <h4 style="margin: 0 0 0.5rem 0; color: #16a34a;">Working (${working.length})</h4>
                <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                    ${working.slice(0, 5).map(a => `<span class="small">${escapeHtml(a.name || a.id)}</span>`).join('')}
                    ${working.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${working.length - 5} more</span>` : ''}
                </div>
            </div>
            <div style="padding: 1rem; border: 1px solid #f59e0b; border-radius: 4px; background: rgba(245, 158, 11, 0.05);">
                <h4 style="margin: 0 0 0.5rem 0; color: #f59e0b;">Idle (${idle.length})</h4>
                <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                    ${idle.slice(0, 5).map(a => `<span class="small">${escapeHtml(a.name || a.id)}</span>`).join('')}
                    ${idle.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${idle.length - 5} more</span>` : ''}
                </div>
            </div>
            <div style="padding: 1rem; border: 1px solid #dc2626; border-radius: 4px; background: rgba(220, 38, 38, 0.05);">
                <h4 style="margin: 0 0 0.5rem 0; color: #dc2626;">Blocked (${blocked.length})</h4>
                <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                    ${blocked.slice(0, 5).map(a => `<span class="small">${escapeHtml(a.name || a.id)}</span>`).join('')}
                    ${blocked.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${blocked.length - 5} more</span>` : ''}
                </div>
            </div>
            <div style="padding: 1rem; border: 1px solid #6b7280; border-radius: 4px; background: rgba(107, 114, 128, 0.05);">
                <h4 style="margin: 0 0 0.5rem 0; color: #6b7280;">Vacant Positions (${vacant.length})</h4>
                <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                    ${vacant.slice(0, 5).map(p => `<span class="small">${escapeHtml(p.title || p.id)}</span>`).join('')}
                    ${vacant.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${vacant.length - 5} more</span>` : ''}
                </div>
            </div>
        </div>
    `;
}

// Render Review Summary
function renderReviewSummary() {
    const container = document.getElementById('review-summary-container');
    if (!container) return;
    
    const summary = state.reviewSummary || {};
    const grades = summary.grade_distribution || {};
    const warning = summary.agents_on_warning || [];
    const risk = summary.agents_at_risk || [];
    
    container.innerHTML = `
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 2rem;">
            <div>
                <h4>Grade Distribution</h4>
                <div id="review-grades-chart" style="width: 100%; height: 200px;"></div>
            </div>
            <div>
                <h4>Agents on Warning (${warning.length})</h4>
                <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                    ${warning.slice(0, 5).map(a => `
                        <div style="padding: 0.5rem; background: rgba(245, 158, 11, 0.1); border-radius: 4px;">
                            <span class="small"><strong>${escapeHtml(a.name || a.id)}</strong></span>
                            <span class="small" style="color: var(--text-muted);"> - Grade: ${a.grade}</span>
                        </div>
                    `).join('')}
                    ${warning.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${warning.length - 5} more</span>` : ''}
                </div>
            </div>
        </div>
        <div style="margin-top: 1rem;">
            <h4>Agents at Risk of Firing (${risk.length})</h4>
            <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                ${risk.slice(0, 5).map(a => `
                    <div style="padding: 0.5rem; background: rgba(220, 38, 38, 0.1); border-radius: 4px;">
                        <span class="small"><strong>${escapeHtml(a.name || a.id)}</strong></span>
                        <span class="small" style="color: var(--text-muted);"> - Grade: ${a.grade}</span>
                    </div>
                `).join('')}
                ${risk.length > 5 ? `<span class="small" style="color: var(--text-muted);">+${risk.length - 5} more</span>` : ''}
            </div>
        </div>
    `;
}

// Render Escalation Queue
function renderEscalationQueue() {
    const container = document.getElementById('escalation-queue-container');
    if (!container) return;
    
    const escalations = (state.decisions || []).filter(d => d.requires_human === true);
    if (escalations.length === 0) {
        container.innerHTML = renderEmptyState('No escalations', 'Human decisions will appear here.');
        return;
    }
    
    container.innerHTML = `
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 1rem;">
            ${escalations.map(e => `
                <div style="padding: 1rem; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary);">
                    <div style="display: flex; justify-content: space-between; align-items: start; margin-bottom: 0.5rem;">
                        <h4 style="margin: 0;">${escapeHtml(e.question || e.title || 'Decision')}</h4>
                        <span class="badge priority-${e.priority || 2}" style="white-space: nowrap;">P${e.priority || 2}</span>
                    </div>
                    <p class="small" style="margin: 0.5rem 0; color: var(--text-muted);"><strong>From:</strong> ${escapeHtml(e.requester_id || 'Unknown')}</p>
                    ${e.recommendation ? `<p class="small" style="margin: 0.5rem 0;"><strong>Recommendation:</strong> ${escapeHtml(e.recommendation)}</p>` : ''}
                    <button type="button" class="primary" onclick="viewDecision('${escapeHtml(e.id)}')" style="margin-top: 0.5rem; width: 100%;">Review & Decide</button>
                </div>
            `).join('')}
        </div>
    `;
}
