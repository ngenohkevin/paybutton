// Admin Dashboard JavaScript Functions

// Quick action functions
function refillPool() {
    if (confirm('Are you sure you want to manually refill the address pool?')) {
        fetch('/admin/pool/refill', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            }
        })
        .then(response => response.json())
        .then(data => {
            showNotification('success', 'Pool refill initiated successfully');
            // Trigger dashboard refresh
            htmx.trigger('#auto-refresh', 'refresh');
        })
        .catch(error => {
            showNotification('error', 'Error initiating pool refill: ' + error.message);
        });
    }
}

function resetGap() {
    const count = prompt('Enter the new unpaid address count:', '0');
    if (count !== null && !isNaN(count)) {
        fetch('/admin/gap/reset', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ count: parseInt(count) })
        })
        .then(response => response.json())
        .then(data => {
            showNotification('success', 'Gap counter reset successfully');
            htmx.trigger('#auto-refresh', 'refresh');
        })
        .catch(error => {
            showNotification('error', 'Error resetting gap counter: ' + error.message);
        });
    }
}

function viewLogs() {
    // Navigate to logs page in the same tab
    window.location.href = '/admin/logs';
}

function exportStats() {
    fetch('/admin/export/stats')
        .then(response => response.blob())
        .then(blob => {
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.style.display = 'none';
            a.href = url;
            a.download = 'paybutton-stats-' + new Date().toISOString().slice(0, 10) + '.json';
            document.body.appendChild(a);
            a.click();
            window.URL.revokeObjectURL(url);
            document.body.removeChild(a);
            showNotification('success', 'Stats exported successfully');
        })
        .catch(error => {
            showNotification('error', 'Error exporting stats: ' + error.message);
        });
}

// Notification system
function showNotification(type, message) {
    const notification = document.createElement('div');
    notification.className = `fixed top-4 right-4 px-6 py-4 rounded-lg shadow-lg z-50 transition-all duration-300 transform translate-x-full`;
    
    if (type === 'success') {
        notification.className += ' bg-green-500 text-white';
        notification.innerHTML = `<i class="fas fa-check-circle mr-2"></i>${message}`;
    } else if (type === 'error') {
        notification.className += ' bg-red-500 text-white';
        notification.innerHTML = `<i class="fas fa-exclamation-circle mr-2"></i>${message}`;
    } else {
        notification.className += ' bg-blue-500 text-white';
        notification.innerHTML = `<i class="fas fa-info-circle mr-2"></i>${message}`;
    }
    
    document.body.appendChild(notification);
    
    // Animate in
    setTimeout(() => {
        notification.classList.remove('translate-x-full');
    }, 100);
    
    // Auto dismiss after 5 seconds
    setTimeout(() => {
        notification.classList.add('translate-x-full');
        setTimeout(() => {
            if (document.body.contains(notification)) {
                document.body.removeChild(notification);
            }
        }, 300);
    }, 5000);
    
    // Click to dismiss
    notification.addEventListener('click', () => {
        notification.classList.add('translate-x-full');
        setTimeout(() => {
            if (document.body.contains(notification)) {
                document.body.removeChild(notification);
            }
        }, 300);
    });
}

// Real-time status indicators
function updateStatusIndicators(data) {
    // Update system status
    const statusIndicator = document.getElementById('system-status');
    if (statusIndicator && data.status) {
        statusIndicator.textContent = data.status.toUpperCase();
        statusIndicator.className = data.status === 'healthy' 
            ? 'text-3xl font-bold text-green-600 mb-2'
            : 'text-3xl font-bold text-red-600 mb-2';
    }
    
    // Update charts if they exist
    if (window.updateCharts && typeof window.updateCharts === 'function') {
        window.updateCharts(data);
    }
}

// Keyboard shortcuts
document.addEventListener('keydown', function(e) {
    // Ctrl/Cmd + R: Refresh dashboard
    if ((e.ctrlKey || e.metaKey) && e.key === 'r') {
        e.preventDefault();
        htmx.trigger('#auto-refresh', 'refresh');
        showNotification('info', 'Dashboard refreshed');
    }
    
    // Ctrl/Cmd + P: Refill pool
    if ((e.ctrlKey || e.metaKey) && e.key === 'p') {
        e.preventDefault();
        refillPool();
    }
    
    // Ctrl/Cmd + G: Reset gap
    if ((e.ctrlKey || e.metaKey) && e.key === 'g') {
        e.preventDefault();
        resetGap();
    }
});

// Enhanced Connection status monitoring with better error handling
let wsConnection = null;
let reconnectAttempts = 0;
const maxReconnectAttempts = 3;
let reconnectTimer = null;
let isConnecting = false;

function initWebSocket() {
    // Prevent multiple connection attempts
    if (isConnecting || (wsConnection && wsConnection.readyState === WebSocket.CONNECTING)) {
        return;
    }
    
    isConnecting = true;
    
    // Make wsConnection globally available
    window.wsConnection = wsConnection;
    
    try {
        // Clean up existing connection
        if (wsConnection) {
            wsConnection.close();
            wsConnection = null;
        }
        
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/admin/ws`;
        
        wsConnection = new WebSocket(wsUrl);
        
        wsConnection.onopen = function() {
            isConnecting = false;
            reconnectAttempts = 0;
            if (reconnectTimer) {
                clearTimeout(reconnectTimer);
                reconnectTimer = null;
            }
            // Only show success notification after reconnection (not on initial connection)
            if (reconnectAttempts > 0) {
                showNotification('success', 'Real-time monitoring reconnected');
            }
        };
        
        wsConnection.onmessage = function(event) {
            try {
                // Check if message is empty or just whitespace
                if (!event.data || event.data.trim() === '') {
                    return;
                }
                
                const data = JSON.parse(event.data);
                
                // Validate data structure
                if (data && typeof data === 'object') {
                    // Update charts directly with WebSocket data
                    if (window.updateCharts && typeof window.updateCharts === 'function') {
                        window.updateCharts(data);
                    }
                    
                    // Update status indicators without triggering full refresh
                    if (data.type === 'status_update' || data.status) {
                        updateStatusIndicators(data);
                    }
                    
                    // Don't trigger HTMX refresh - let WebSocket handle real-time updates
                }
            } catch (error) {
                // Only log parsing errors if the message isn't empty
                if (event.data && event.data.trim() !== '') {
                    console.warn('Invalid WebSocket message received:', event.data);
                }
            }
        };
        
        wsConnection.onclose = function(event) {
            isConnecting = false;
            wsConnection = null;
            
            // Only attempt reconnection for unexpected closures
            if (event.code !== 1000 && reconnectAttempts < maxReconnectAttempts) {
                reconnectAttempts++;
                const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 10000); // Exponential backoff, max 10s
                
                reconnectTimer = setTimeout(() => {
                    initWebSocket();
                }, delay);
            } else if (reconnectAttempts >= maxReconnectAttempts) {
                console.warn('WebSocket connection failed after maximum retry attempts');
            }
        };
        
        wsConnection.onerror = function(error) {
            isConnecting = false;
            // Reduce error noise - only log significant errors
            if (reconnectAttempts === 0) {
                console.warn('WebSocket connection error - will attempt to reconnect');
            }
        };
        
        // Set connection timeout
        setTimeout(() => {
            if (wsConnection && wsConnection.readyState === WebSocket.CONNECTING) {
                wsConnection.close();
                isConnecting = false;
            }
        }, 5000);
        
    } catch (error) {
        isConnecting = false;
        console.error('Failed to initialize WebSocket:', error);
    }
}

// Cleanup WebSocket connection
function cleanupWebSocket() {
    if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }
    if (wsConnection) {
        wsConnection.close(1000, 'Page unloading');
        wsConnection = null;
    }
    isConnecting = false;
    reconnectAttempts = 0;
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', function() {
    // Initialize WebSocket for real-time updates
    initWebSocket();
    
    // Add keyboard shortcut hints
    const shortcuts = document.createElement('div');
    shortcuts.innerHTML = `
        <div class="fixed bottom-4 left-4 bg-gray-800 text-white text-xs px-3 py-2 rounded-lg opacity-75 hover:opacity-100 transition-opacity">
            <div class="font-semibold mb-1">Keyboard Shortcuts:</div>
            <div>Ctrl+R: Refresh • Ctrl+P: Refill Pool • Ctrl+G: Reset Gap</div>
        </div>
    `;
    document.body.appendChild(shortcuts);
    
    // Hide shortcuts after 10 seconds
    setTimeout(() => {
        shortcuts.style.opacity = '0';
        setTimeout(() => {
            if (document.body.contains(shortcuts)) {
                document.body.removeChild(shortcuts);
            }
        }, 300);
    }, 10000);
});

// Enhanced HTMX error handling
if (document.body) {
    document.body.addEventListener('htmx:responseError', function(event) {
        if (!event.detail || !event.detail.xhr) {
            showNotification('error', 'Network request failed');
            return;
        }
        
        const status = event.detail.xhr.status;
    const statusText = event.detail.xhr.statusText;
    
    if (status === 401) {
        showNotification('error', 'Session expired. Please log in again.');
        setTimeout(() => {
            window.location.href = '/admin/login';
        }, 2000);
    } else {
        showNotification('error', `Request failed: ${status} ${statusText}`);
    }
    });
}

// Enhanced HTMX success handling
if (document.body) {
    document.body.addEventListener('htmx:afterSwap', function(event) {
        if (event.target.id === 'dashboard-content') {
            // Dashboard was refreshed successfully
            const timestamp = new Date().toLocaleTimeString();
            console.log(`Dashboard refreshed at ${timestamp}`);
            
            // Initialize charts after HTMX content loads
            if (typeof initializeCharts === 'function') {
                setTimeout(initializeCharts, 100);
            }
        }
    });
}

// Cleanup WebSocket on page unload
window.addEventListener('beforeunload', cleanupWebSocket);
window.addEventListener('unload', cleanupWebSocket);

// Handle visibility changes to pause/resume connections
document.addEventListener('visibilitychange', function() {
    if (document.hidden) {
        // Page is hidden, cleanup WebSocket
        cleanupWebSocket();
    } else {
        // Page is visible again, reinitialize if needed
        if (!wsConnection || wsConnection.readyState === WebSocket.CLOSED) {
            setTimeout(initWebSocket, 1000);
        }
    }
});