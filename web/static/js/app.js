// Main application entry point
import { auth, aircraft, telescope, system, showToast } from './api.js';

/**
 * Application state
 */
const state = {
    map: null,
    aircraftMarkers: {},
    observerMarker: null,
    selectedAircraft: null,
    altitudeChart: null,
    updateInterval: null,
    activeObserver: null,
    aircraftData: [], // Cache of current aircraft data
    telescopeConfig: null, // Telescope configuration and capabilities
};

/**
 * Initialize the application
 */
async function init() {
    console.log('Initializing ADS-B Scope PWA...');
    
    // Check if user is already logged in
    if (auth.isAuthenticated()) {
        showAppScreen();
    } else {
        showLoginScreen();
    }
    
    // Setup event listeners
    setupEventListeners();
}

/**
 * Setup all event listeners
 */
function setupEventListeners() {
    // Login form
    document.getElementById('login-form')?.addEventListener('submit', handleLogin);
    
    // Logout button
    document.getElementById('btn-logout')?.addEventListener('click', handleLogout);
    
    // Telescope controls
    document.getElementById('btn-start-tracking')?.addEventListener('click', handleStartTracking);
    document.getElementById('btn-stop-tracking')?.addEventListener('click', handleStopTracking);
    document.getElementById('btn-abort')?.addEventListener('click', handleAbort);
    
    // Manual slew buttons
    document.querySelectorAll('.btn-slew:not(.btn-stop)').forEach(btn => {
        btn.addEventListener('click', (e) => {
            const direction = e.target.dataset.direction;
            if (direction) handleManualSlew(direction);
        });
    });
    
    // Map controls
    document.getElementById('btn-center-telescope')?.addEventListener('click', centerOnTelescope);
    
    // Aircraft search
    document.getElementById('aircraft-search')?.addEventListener('input', filterAircraft);
    document.getElementById('aircraft-sort')?.addEventListener('change', sortAircraft);
}

/**
 * Handle login form submission
 */
async function handleLogin(e) {
    e.preventDefault();
    
    const username = document.getElementById('username-input').value;
    const password = document.getElementById('password-input').value;
    const errorEl = document.getElementById('login-error');
    
    try {
        const result = await auth.login(username, password);
        console.log('Login successful:', result.user);
        showToast(`Welcome, ${result.user.username}!`, 'success');
        showAppScreen();
    } catch (error) {
        console.error('Login failed:', error);
        errorEl.textContent = error.message;
        errorEl.classList.remove('hidden');
    }
}

/**
 * Handle logout
 */
async function handleLogout() {
    try {
        await auth.logout();
        showToast('Logged out successfully', 'info');
        showLoginScreen();
    } catch (error) {
        console.error('Logout failed:', error);
        showToast('Logout failed', 'error');
    }
}

/**
 * Show login screen
 */
function showLoginScreen() {
    document.getElementById('login-screen').classList.remove('hidden');
    document.getElementById('app-screen').classList.add('hidden');
    document.getElementById('btn-login').classList.remove('hidden');
    document.getElementById('user-menu').classList.add('hidden');
    
    // Stop updates
    if (state.updateInterval) {
        clearInterval(state.updateInterval);
        state.updateInterval = null;
    }
}

/**
 * Show app screen and initialize components
 */
async function showAppScreen() {
    const user = auth.getCurrentUser();
    
    document.getElementById('login-screen').classList.add('hidden');
    document.getElementById('app-screen').classList.remove('hidden');
    document.getElementById('btn-login').classList.add('hidden');
    document.getElementById('user-menu').classList.remove('hidden');
    document.getElementById('username').textContent = user.username;
    document.getElementById('control-role').textContent = user.role;
    
    // Load active observation point first
    await loadActiveObserver();
    
    // Load telescope configuration
    await loadTelescopeConfig();
    
    // Initialize components
    initMap();
    initChart();
    startUpdates();
}

/**
 * Initialize Leaflet map
 */
function initMap() {
    const mapEl = document.getElementById('sky-map');
    
    // Get observer location (default to config location if not loaded)
    const observerLat = state.activeObserver?.latitude || 37.1401;
    const observerLon = state.activeObserver?.longitude || -94.4912;
    
    // Initialize map centered on observer location
    state.map = L.map(mapEl, {
        center: [observerLat, observerLon],
        zoom: 10,
        zoomControl: true,
    });
    
    // Add dark tile layer
    L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; OpenStreetMap contributors &copy; CARTO',
        subdomains: 'abcd',
        maxZoom: 19,
    }).addTo(state.map);
    
    // Add observer marker
    const observerIcon = L.divIcon({
        className: 'observer-marker',
        html: '<div style="font-size: 24px;">🔭</div>',
        iconSize: [30, 30],
        iconAnchor: [15, 15],
    });
    
    state.observerMarker = L.marker([observerLat, observerLon], { icon: observerIcon })
        .addTo(state.map)
        .bindPopup(state.activeObserver?.name || 'Observer');
    
    // Add click handler to create new observation points (TODO: implement)
    // state.map.on('click', handleMapClick);
}

/**
 * Initialize altitude chart
 */
function initChart() {
    const ctx = document.getElementById('altitude-chart');
    if (!ctx) return;
    
    // Use telescope config limits if available, otherwise defaults
    const minAlt = state.telescopeConfig?.minAltitude ?? 0;
    const maxAlt = state.telescopeConfig?.maxAltitude ?? 85;
    
    state.altitudeChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [{
                label: 'Altitude',
                data: [],
                borderColor: '#3b82f6',
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                tension: 0.4,
                fill: true,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: false,
                },
            },
            scales: {
                y: {
                    min: minAlt,
                    max: maxAlt,
                    grid: {
                        color: 'rgba(255, 255, 255, 0.1)',
                    },
                    ticks: {
                        color: '#a1a1aa',
                    },
                },
                x: {
                    grid: {
                        color: 'rgba(255, 255, 255, 0.1)',
                    },
                    ticks: {
                        color: '#a1a1aa',
                        maxTicksLimit: 10,
                    },
                },
            },
        },
    });
}

/**
 * Start periodic updates
 */
function startUpdates() {
    // Initial update
    updateAll();
    
    // Update every 2 seconds
    state.updateInterval = setInterval(updateAll, 2000);
}

/**
 * Update all data
 */
async function updateAll() {
    try {
        await Promise.all([
            updateAircraft(),
            updateTelescope(),
            updateSystemStatus(),
        ]);
    } catch (error) {
        console.error('Update failed:', error);
    }
}

/**
 * Load active observation point
 */
async function loadActiveObserver() {
    try {
        const observer = await fetch('/api/v1/observer/active', {
            headers: {
                'Authorization': `Bearer ${sessionStorage.getItem('authToken')}`
            }
        });
        
        if (observer.ok) {
            state.activeObserver = await observer.json();
            console.log('Loaded active observer:', state.activeObserver);
        }
    } catch (error) {
        console.error('Failed to load active observer:', error);
    }
}

/**
 * Load telescope configuration
 */
async function loadTelescopeConfig() {
    try {
        const response = await fetch('/api/v1/telescope/config', {
            headers: {
                'Authorization': `Bearer ${sessionStorage.getItem('authToken')}`
            }
        });
        
        if (response.ok) {
            state.telescopeConfig = await response.json();
            console.log('Loaded telescope config:', state.telescopeConfig);
            
            // Update chart limits if chart exists
            if (state.altitudeChart) {
                state.altitudeChart.options.scales.y.min = state.telescopeConfig.minAltitude;
                state.altitudeChart.options.scales.y.max = state.telescopeConfig.maxAltitude;
            }
        }
    } catch (error) {
        console.error('Failed to load telescope config:', error);
        // Set defaults if loading fails
        state.telescopeConfig = { minAltitude: 0, maxAltitude: 85 };
    }
}

/**
 * Update aircraft data and map markers
 */
async function updateAircraft() {
    const aircraftData = await aircraft.getAll();
    
    // Update observer location if changed
    if (aircraftData.observer && state.activeObserver) {
        if (state.activeObserver.latitude !== aircraftData.observer.latitude ||
            state.activeObserver.longitude !== aircraftData.observer.longitude) {
            state.activeObserver = aircraftData.observer;
            if (state.observerMarker) {
                state.observerMarker.setLatLng([
                    aircraftData.observer.latitude,
                    aircraftData.observer.longitude
                ]);
            }
        }
    }
    
    // Track which aircraft we've seen in this update
    const currentAircraft = new Set(aircraftData.map(ac => ac.icao));
    
    // Remove markers for aircraft that are no longer visible
    Object.keys(state.aircraftMarkers).forEach(icao => {
        if (!currentAircraft.has(icao)) {
            state.aircraftMarkers[icao].remove();
            delete state.aircraftMarkers[icao];
        }
    });
    
    // Update markers on map
    aircraftData.forEach(ac => {
        if (!state.aircraftMarkers[ac.icao]) {
            // Create new marker
            const icon = L.divIcon({
                className: 'aircraft-marker',
                html: `<div style="font-size: 20px; transform: rotate(${ac.heading}deg);">✈️</div>`,
                iconSize: [24, 24],
                iconAnchor: [12, 12],
            });
            
            const marker = L.marker([ac.lat, ac.lon], { icon })
                .bindPopup(`${ac.callsign} - ${ac.altitude}ft`)
                .addTo(state.map);
            
            marker.on('click', () => selectAircraft(ac.icao));
            
            state.aircraftMarkers[ac.icao] = marker;
        } else {
            // Update existing marker position and rotation
            const marker = state.aircraftMarkers[ac.icao];
            marker.setLatLng([ac.lat, ac.lon]);
            
            // Update the icon with new rotation
            const icon = L.divIcon({
                className: 'aircraft-marker',
                html: `<div style="font-size: 20px; transform: rotate(${ac.heading}deg);">✈️</div>`,
                iconSize: [24, 24],
                iconAnchor: [12, 12],
            });
            marker.setIcon(icon);
            
            // Update popup
            marker.setPopupContent(`${ac.callsign} - ${ac.altitude}ft`);
        }
    });
    
    // Cache aircraft data for selection
    state.aircraftData = aircraftData;
    
    // Update aircraft list
    updateAircraftList(aircraftData);
}

/**
 * Update aircraft list display
 */
function updateAircraftList(aircraftData) {
    const listEl = document.getElementById('aircraft-list');
    if (!listEl) return;
    
    listEl.innerHTML = aircraftData.map(ac => `
        <div class="aircraft-item ${state.selectedAircraft === ac.icao ? 'selected' : ''}" 
             data-icao="${ac.icao}"
             onclick="window.selectAircraft('${ac.icao}')">
            <div class="aircraft-header">
                <span class="aircraft-id">${ac.callsign}</span>
                <span class="aircraft-distance">${ac.distance.toFixed(1)} km</span>
            </div>
            <div class="aircraft-details">
                <div class="aircraft-detail">
                    <span class="aircraft-detail-label">Alt</span>
                    <span class="aircraft-detail-value">${(ac.altitude / 1000).toFixed(1)}k ft</span>
                </div>
                <div class="aircraft-detail">
                    <span class="aircraft-detail-label">Speed</span>
                    <span class="aircraft-detail-value">${ac.speed} kts</span>
                </div>
                <div class="aircraft-detail">
                    <span class="aircraft-detail-label">Elev</span>
                    <span class="aircraft-detail-value">${ac.elevation.toFixed(1)}°</span>
                </div>
            </div>
        </div>
    `).join('');
}

/**
 * Select an aircraft
 */
function selectAircraft(icao) {
    state.selectedAircraft = icao;
    
    try {
        // Find aircraft in cached data
        const ac = state.aircraftData.find(a => a.icao === icao);
        
        if (!ac) {
            showToast('Aircraft not found', 'error');
            return;
        }
        
        // Update target info
        const targetInfo = document.getElementById('target-info');
        targetInfo.innerHTML = `
            <div class="target-data">
                <div class="target-row">
                    <span class="target-label">Callsign:</span>
                    <span class="target-value">${ac.callsign}</span>
                </div>
                <div class="target-row">
                    <span class="target-label">Altitude:</span>
                    <span class="target-value">${ac.altitude.toLocaleString()} ft</span>
                </div>
                <div class="target-row">
                    <span class="target-label">Distance:</span>
                    <span class="target-value">${ac.distance.toFixed(1)} km</span>
                </div>
                <div class="target-row">
                    <span class="target-label">Azimuth:</span>
                    <span class="target-value">${ac.azimuth.toFixed(1)}°</span>
                </div>
                <div class="target-row">
                    <span class="target-label">Elevation:</span>
                    <span class="target-value">${ac.elevation.toFixed(1)}°</span>
                </div>
            </div>
        `;
        
        // Enable tracking button
        document.getElementById('btn-start-tracking').disabled = false;
        
        // Center map on aircraft
        if (state.map && state.aircraftMarkers[icao]) {
            state.map.setView([ac.lat, ac.lon], 12);
        }
        
        showToast(`Selected ${ac.callsign}`, 'info');
    } catch (error) {
        console.error('Failed to select aircraft:', error);
        showToast('Failed to select aircraft', 'error');
    }
}

// Make selectAircraft available globally for onclick handlers
window.selectAircraft = selectAircraft;

/**
 * Update telescope telemetry
 */
async function updateTelescope() {
    const status = await telescope.getStatus();
    
    // Update telemetry display
    document.getElementById('tel-altaz').textContent = 
        `${status.altitude.toFixed(1)}° / ${status.azimuth.toFixed(1)}°`;
    document.getElementById('tel-radec').textContent = 
        status.rightAscension != null && status.declination != null 
            ? `${status.rightAscension.toFixed(1)}h / ${status.declination.toFixed(1)}°`
            : 'N/A';
    document.getElementById('tel-state').textContent = 
        status.tracking ? 'Tracking' : status.slewing ? 'Slewing' : 'Idle';
    document.getElementById('tel-slewing').textContent = status.slewing ? 'Yes' : 'No';
    
    // Update altitude chart
    if (state.altitudeChart) {
        const now = new Date().toLocaleTimeString();
        state.altitudeChart.data.labels.push(now);
        state.altitudeChart.data.datasets[0].data.push(status.altitude);
        
        // Keep only last 20 points
        if (state.altitudeChart.data.labels.length > 20) {
            state.altitudeChart.data.labels.shift();
            state.altitudeChart.data.datasets[0].data.shift();
        }
        
        state.altitudeChart.update('none');
    }
    
    // Update warning
    const warningEl = document.getElementById('tel-warning');
    const minAlt = state.telescopeConfig?.minAltitude ?? 0;
    const maxAlt = state.telescopeConfig?.maxAltitude ?? 85;
    const warnThreshold = 5; // Warn 5° before limit
    
    if (status.altitude > maxAlt) {
        warningEl.textContent = 'Limit Exceeded!';
        warningEl.className = 'value warning-exceeded';
    } else if (status.altitude > (maxAlt - warnThreshold)) {
        warningEl.textContent = 'Approaching Upper Limit';
        warningEl.className = 'value warning-approaching';
    } else if (status.altitude < minAlt) {
        warningEl.textContent = 'Limit Exceeded!';
        warningEl.className = 'value warning-exceeded';
    } else if (status.altitude < (minAlt + warnThreshold)) {
        warningEl.textContent = 'Approaching Lower Limit';
        warningEl.className = 'value warning-approaching';
    } else {
        warningEl.textContent = 'None';
        warningEl.className = 'value warning-none';
    }
}

/**
 * Update system status indicators
 */
async function updateSystemStatus() {
    const status = await system.getStatus();
    
    document.getElementById('status-telescope').className = 
        `status-dot ${status.telescope ? 'connected' : 'error'}`;
    document.getElementById('status-adsb').className = 
        `status-dot ${status.adsb ? 'connected' : 'error'}`;
    document.getElementById('status-tracking').className = 
        `status-dot ${status.tracking ? 'tracking' : ''}`;
}

/**
 * Handle start tracking
 */
async function handleStartTracking() {
    if (!state.selectedAircraft) {
        showToast('Please select an aircraft first', 'error');
        return;
    }
    
    try {
        await telescope.startTracking(state.selectedAircraft);
        
        document.getElementById('btn-start-tracking').classList.add('hidden');
        document.getElementById('btn-stop-tracking').classList.remove('hidden');
        
        showToast('Tracking started', 'success');
    } catch (error) {
        console.error('Failed to start tracking:', error);
        // Show the specific error message from the API
        const errorMsg = error.message || 'Failed to start tracking';
        showToast(errorMsg, 'error');
    }
}

/**
 * Handle stop tracking
 */
async function handleStopTracking() {
    try {
        await telescope.stopTracking();
        
        document.getElementById('btn-start-tracking').classList.remove('hidden');
        document.getElementById('btn-stop-tracking').classList.add('hidden');
        
        showToast('Tracking stopped', 'info');
    } catch (error) {
        console.error('Failed to stop tracking:', error);
        showToast('Failed to stop tracking', 'error');
    }
}

/**
 * Handle abort/emergency stop
 */
async function handleAbort() {
    try {
        await telescope.abort();
        
        document.getElementById('btn-start-tracking').classList.remove('hidden');
        document.getElementById('btn-stop-tracking').classList.add('hidden');
        
        showToast('Telescope stopped', 'info');
    } catch (error) {
        console.error('Failed to abort:', error);
        showToast('Failed to stop telescope', 'error');
    }
}

/**
 * Handle manual slew
 */
async function handleManualSlew(direction) {
    try {
        await telescope.slewDirection(direction);
    } catch (error) {
        console.error('Slew failed:', error);
        showToast('Slew failed', 'error');
    }
}

/**
 * Center map on observer
 */
function centerOnTelescope() {
    if (state.observerMarker && state.map) {
        state.map.setView(state.observerMarker.getLatLng(), 10);
    }
}

/**
 * Filter aircraft list
 */
function filterAircraft() {
    const searchTerm = document.getElementById('aircraft-search').value.toLowerCase();
    const items = document.querySelectorAll('.aircraft-item');
    
    items.forEach(item => {
        const text = item.textContent.toLowerCase();
        item.style.display = text.includes(searchTerm) ? '' : 'none';
    });
}

/**
 * Sort aircraft list
 */
function sortAircraft() {
    // This would need to trigger a re-fetch with sort parameter
    // For now, just show a toast
    const sortBy = document.getElementById('aircraft-sort').value;
    showToast(`Sorting by ${sortBy}`, 'info');
}

// Initialize on DOMContentLoaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
