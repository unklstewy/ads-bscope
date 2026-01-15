// API client for ADS-B Scope backend

/**
 * Base API configuration
 */
const API_BASE = '/api/v1';

/**
 * Authentication state
 */
let currentUser = null;
let authToken = null;


/**
 * Helper to make authenticated API requests
 */
async function apiRequest(endpoint, options = {}) {
    const url = `${API_BASE}${endpoint}`;
    const headers = {
        'Content-Type': 'application/json',
        ...options.headers,
    };
    
    // Add auth token if available
    if (authToken) {
        headers['Authorization'] = `Bearer ${authToken}`;
    }
    
    const response = await fetch(url, {
        ...options,
        headers,
    });
    
    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || `HTTP ${response.status}: ${response.statusText}`);
    }
    
    return response.json();
}

/**
 * Authentication API
 */
export const auth = {
    async login(username, password) {
        const response = await apiRequest('/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password }),
        });
        
        if (response.success && response.token) {
            authToken = response.token;
            currentUser = response.user;
            
            // Store in sessionStorage for persistence
            sessionStorage.setItem('authToken', authToken);
            sessionStorage.setItem('currentUser', JSON.stringify(currentUser));
            
            return response;
        }
        
        throw new Error('Login failed');
    },
    
    async logout() {
        try {
            await apiRequest('/auth/logout', { method: 'POST' });
        } finally {
            // Clear state even if API call fails
            currentUser = null;
            authToken = null;
            sessionStorage.removeItem('authToken');
            sessionStorage.removeItem('currentUser');
        }
        return { success: true };
    },
    
    getCurrentUser() {
        // Try to restore from sessionStorage
        if (!currentUser) {
            const stored = sessionStorage.getItem('currentUser');
            if (stored) {
                currentUser = JSON.parse(stored);
                authToken = sessionStorage.getItem('authToken');
            }
        }
        return currentUser;
    },
    
    isAuthenticated() {
        return !!this.getCurrentUser();
    },
};

/**
 * Aircraft API
 */
export const aircraft = {
    async getAll() {
        const response = await apiRequest('/aircraft');
        return response.aircraft || [];
    },
    
    async getById(icao) {
        return await apiRequest(`/aircraft/${icao}`);
    },
};

/**
 * Telescope API
 */
export const telescope = {
    async getStatus() {
        return await apiRequest('/telescope/status');
    },
    
    async slew(altitude, azimuth) {
        return await apiRequest('/telescope/slew', {
            method: 'POST',
            body: JSON.stringify({ altitude, azimuth }),
        });
    },
    
    async slewDirection(direction) {
        // Get current status to calculate new position
        const status = await this.getStatus();
        const increment = 1.0;
        
        let newAlt = status.altitude;
        let newAz = status.azimuth;
        
        switch (direction) {
            case 'n':
                newAlt = Math.min(80, status.altitude + increment);
                break;
            case 's':
                newAlt = Math.max(20, status.altitude - increment);
                break;
            case 'e':
                newAz = (status.azimuth + increment) % 360;
                break;
            case 'w':
                newAz = (status.azimuth - increment + 360) % 360;
                break;
            case 'ne':
                newAlt = Math.min(80, status.altitude + increment);
                newAz = (status.azimuth + increment) % 360;
                break;
            case 'nw':
                newAlt = Math.min(80, status.altitude + increment);
                newAz = (status.azimuth - increment + 360) % 360;
                break;
            case 'se':
                newAlt = Math.max(20, status.altitude - increment);
                newAz = (status.azimuth + increment) % 360;
                break;
            case 'sw':
                newAlt = Math.max(20, status.altitude - increment);
                newAz = (status.azimuth - increment + 360) % 360;
                break;
        }
        
        return await this.slew(newAlt, newAz);
    },
    
    async abort() {
        return await apiRequest('/telescope/abort', {
            method: 'POST',
        });
    },
    
    async startTracking(icao) {
        return await apiRequest(`/telescope/track/${icao}`, {
            method: 'POST',
        });
    },
    
    async stopTracking() {
        return await apiRequest('/telescope/stop', {
            method: 'POST',
        });
    },
};

/**
 * Observer (observation point) API
 */
export const observer = {
    async getPoints() {
        const response = await apiRequest('/observer/points');
        return response.points || [];
    },
    
    async getActive() {
        return await apiRequest('/observer/active');
    },
    
    async create(point) {
        return await apiRequest('/observer/points', {
            method: 'POST',
            body: JSON.stringify(point),
        });
    },
    
    async update(id, point) {
        return await apiRequest(`/observer/points/${id}`, {
            method: 'PUT',
            body: JSON.stringify(point),
        });
    },
    
    async delete(id) {
        return await apiRequest(`/observer/points/${id}`, {
            method: 'DELETE',
        });
    },
    
    async activate(id) {
        return await apiRequest(`/observer/points/${id}/activate`, {
            method: 'POST',
        });
    },
};

/**
 * System status API
 */
export const system = {
    async getStatus() {
        return await apiRequest('/system/status');
    },
};

/**
 * Toast notification helper
 */
export function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    if (!container) return;
    
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    
    container.appendChild(toast);
    
    // Auto-remove after 3 seconds
    setTimeout(() => {
        toast.style.animation = 'slideIn 0.3s ease-out reverse';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}
