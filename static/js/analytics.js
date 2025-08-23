/**
 * PayButton Analytics SDK
 * Privacy-focused, real-time site analytics with Tor support
 * No cookies, localStorage, or IP tracking
 */
(function() {
    'use strict';

    // Configuration
    const CONFIG = {
        HEARTBEAT_INTERVAL: 15000, // 15 seconds
        RECONNECT_DELAYS: [1000, 2000, 4000, 8000, 16000, 30000], // Exponential backoff
        MAX_RECONNECT_ATTEMPTS: 6,
        CONNECTION_TIMEOUT: 10000 // 10 seconds
    };

    // Analytics state
    let websocket = null;
    let reconnectAttempts = 0;
    let heartbeatTimer = null;
    let reconnectTimer = null;
    let sessionId = null;
    let siteName = null;
    let isConnected = false;
    let currentPagePath = null; // Phase 5: Track current page path

    /**
     * Extract site name from script URL parameter or data attribute
     */
    function extractSiteName() {
        // First try to get from script URL parameter
        const scripts = document.getElementsByTagName('script');
        for (let script of scripts) {
            if (script.src && script.src.includes('analytics.js')) {
                console.log('PayButton Analytics: Found script:', script.src);
                try {
                    const url = new URL(script.src);
                    const siteParam = url.searchParams.get('site');
                    if (siteParam) {
                        console.log('PayButton Analytics: Site name from URL param:', siteParam);
                        return siteParam.trim();
                    }
                } catch (e) {
                    console.error('PayButton Analytics: Error parsing script URL:', e);
                }
            }
        }

        // Fallback: try to get from script data attribute
        for (let script of scripts) {
            if (script.src && script.src.includes('analytics.js')) {
                const siteAttr = script.getAttribute('data-site');
                if (siteAttr) {
                    console.log('PayButton Analytics: Site name from data attribute:', siteAttr);
                    return siteAttr.trim();
                }
            }
        }

        // Last fallback: use current hostname (cleaned)
        const hostname = cleanHostname(window.location.hostname);
        console.log('PayButton Analytics: Using hostname as site name:', hostname);
        return hostname;
    }

    /**
     * Clean hostname for use as site name
     */
    function cleanHostname(hostname) {
        return hostname
            .replace(/^www\./, '') // Remove www.
            .replace(/[^a-zA-Z0-9.-]/g, '_') // Replace special chars
            .substring(0, 50); // Limit length
    }

    /**
     * Generate unique session ID
     */
    function generateSessionId() {
        const timestamp = Date.now();
        const random = Math.floor(Math.random() * 10000);
        return `${timestamp}_${random}`;
    }

    /**
     * Phase 5: Get current page path (clean for privacy)
     */
    function getCurrentPagePath() {
        let path = window.location.pathname;
        
        // Clean path for privacy (remove query params and fragments)
        if (path.length > 100) {
            path = path.substring(0, 100); // Limit length
        }
        
        // Remove potentially sensitive patterns
        path = path.replace(/\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, '/[uuid]'); // UUIDs
        path = path.replace(/\/\d{6,}/g, '/[id]'); // Long numeric IDs
        path = path.replace(/\/[A-Za-z0-9+\/=]{20,}/g, '/[token]'); // Base64 tokens
        
        return path || '/';
    }

    /**
     * Phase 5: Get timezone offset (Tor-friendly)
     */
    function getTimezoneOffset() {
        try {
            return new Date().getTimezoneOffset();
        } catch (e) {
            return 0; // Fallback for privacy
        }
    }

    /**
     * Get WebSocket URL with Phase 5 parameters
     */
    function getWebSocketUrl() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = getAnalyticsHost();
        const basePath = `/ws/analytics/${siteName}`;
        
        // Phase 5: Add query parameters for page path and timezone
        const params = new URLSearchParams();
        if (currentPagePath) {
            params.append('path', currentPagePath);
        }
        const tz = getTimezoneOffset();
        if (tz !== 0) {
            params.append('tz', tz.toString());
        }
        
        const queryString = params.toString();
        const fullPath = queryString ? `${basePath}?${queryString}` : basePath;
        
        return `${protocol}//${host}${fullPath}`;
    }

    /**
     * Get analytics host from script source
     */
    function getAnalyticsHost() {
        const scripts = document.getElementsByTagName('script');
        for (let script of scripts) {
            if (script.src && script.src.includes('analytics.js')) {
                try {
                    const url = new URL(script.src);
                    return url.host;
                } catch (e) {
                    console.warn('PayButton Analytics: Invalid script URL');
                }
            }
        }
        return window.location.host; // Fallback to current host
    }

    /**
     * Log debug messages (always log important events)
     */
    function debug(...args) {
        // Always log in console for debugging
        console.log('PayButton Analytics:', ...args);
    }

    /**
     * Start heartbeat timer
     */
    function startHeartbeat() {
        if (heartbeatTimer) {
            clearInterval(heartbeatTimer);
        }

        heartbeatTimer = setInterval(() => {
            if (websocket && websocket.readyState === WebSocket.OPEN) {
                try {
                    websocket.send(JSON.stringify({
                        type: 'heartbeat',
                        timestamp: new Date().toISOString(),
                        sessionId: sessionId
                    }));
                    debug('Heartbeat sent');
                } catch (error) {
                    debug('Heartbeat failed:', error);
                    handleDisconnection();
                }
            }
        }, CONFIG.HEARTBEAT_INTERVAL);
    }

    /**
     * Stop heartbeat timer
     */
    function stopHeartbeat() {
        if (heartbeatTimer) {
            clearInterval(heartbeatTimer);
            heartbeatTimer = null;
        }
    }

    /**
     * Connect to analytics WebSocket
     */
    function connect() {
        // Clean up any existing connection first
        if (websocket) {
            if (websocket.readyState === WebSocket.CONNECTING || websocket.readyState === WebSocket.OPEN) {
                return; // Already connecting or connected
            }
            // Clean up stale websocket
            websocket.close();
            websocket = null;
        }

        if (!siteName) {
            console.warn('PayButton Analytics: No site name specified');
            return;
        }

        // Phase 5: Update current page path before connecting
        currentPagePath = getCurrentPagePath();
        
        debug(`Connecting to analytics for site: ${siteName}, path: ${currentPagePath}`);

        try {
            const wsUrl = getWebSocketUrl();
            console.log(`PayButton Analytics: Attempting connection to ${wsUrl}`);
            websocket = new WebSocket(wsUrl);

            // Connection timeout
            const connectionTimer = setTimeout(() => {
                if (websocket.readyState === WebSocket.CONNECTING) {
                    debug('Connection timeout');
                    websocket.close();
                    handleReconnection();
                }
            }, CONFIG.CONNECTION_TIMEOUT);

            websocket.onopen = function(event) {
                clearTimeout(connectionTimer);
                isConnected = true;
                reconnectAttempts = 0;
                
                // Generate new session ID on each connection
                sessionId = generateSessionId();
                
                // Start heartbeat to keep connection alive
                startHeartbeat();
                
                // Log successful connection
                console.log(`PayButton Analytics: ✅ Connected for ${siteName} (session: ${sessionId})`);
            };

            websocket.onmessage = function(event) {
                try {
                    const data = JSON.parse(event.data);
                    debug('Received message:', data);
                    
                    // Handle initial connection response
                    if (data.status === 'connected') {
                        debug(`Analytics session established: ${data.sessionId}`);
                    }
                } catch (error) {
                    debug('Message parsing error:', error);
                }
            };

            websocket.onclose = function(event) {
                clearTimeout(connectionTimer);
                console.log(`PayButton Analytics: WebSocket closed - code: ${event.code}, reason: ${event.reason || 'none'}`);
                handleDisconnection();
            };

            websocket.onerror = function(error) {
                clearTimeout(connectionTimer);
                console.error('PayButton Analytics: WebSocket error:', error);
                handleDisconnection();
            };

        } catch (error) {
            debug('Connection error:', error);
            handleReconnection();
        }
    }

    /**
     * Handle disconnection
     */
    function handleDisconnection() {
        isConnected = false;
        stopHeartbeat();
        
        if (websocket) {
            websocket.close();
            websocket = null;
        }

        // Only reconnect if the page is still visible
        if (document.visibilityState === 'visible') {
            handleReconnection();
        }
    }

    /**
     * Handle reconnection with exponential backoff
     */
    function handleReconnection() {
        if (reconnectTimer) {
            return; // Already scheduled
        }

        if (reconnectAttempts >= CONFIG.MAX_RECONNECT_ATTEMPTS) {
            debug('Max reconnection attempts reached');
            return;
        }

        const delay = CONFIG.RECONNECT_DELAYS[Math.min(reconnectAttempts, CONFIG.RECONNECT_DELAYS.length - 1)];
        debug(`Reconnecting in ${delay}ms (attempt ${reconnectAttempts + 1})`);

        reconnectTimer = setTimeout(() => {
            reconnectTimer = null;
            reconnectAttempts++;
            connect();
        }, delay);
    }

    /**
     * Phase 5: Track page navigation for SPAs
     */
    function trackPageChange() {
        const newPath = getCurrentPagePath();
        if (newPath !== currentPagePath) {
            debug(`Page changed from ${currentPagePath} to ${newPath}`);
            currentPagePath = newPath;
            
            // Reconnect with new page path
            if (isConnected) {
                handleDisconnection();
                setTimeout(connect, 500);
            }
        }
    }

    /**
     * Initialize analytics tracking
     */
    function initialize() {
        console.log('PayButton Analytics: Starting initialization');
        
        // Extract site name from script parameters
        siteName = extractSiteName();
        
        if (!siteName) {
            console.error('PayButton Analytics: Unable to determine site name - analytics will not work');
            return;
        }

        console.log(`PayButton Analytics: Initializing for site: ${siteName}`);
        
        // Reset connection state on page load
        isConnected = false;
        websocket = null;
        reconnectAttempts = 0;
        sessionId = null;

        // Start connection immediately or when DOM is ready
        if (document.readyState === 'loading') {
            console.log('PayButton Analytics: Waiting for DOM...');
            document.addEventListener('DOMContentLoaded', function() {
                console.log('PayButton Analytics: DOM ready, connecting...');
                connect();
            });
        } else {
            // Page already loaded - connect immediately
            console.log('PayButton Analytics: Page ready, connecting immediately...');
            connect();
        }

        // Phase 5: Track navigation changes (for SPAs)
        // Monitor pushState/replaceState for SPA navigation
        if (window.history && window.history.pushState) {
            const originalPushState = window.history.pushState;
            const originalReplaceState = window.history.replaceState;
            
            window.history.pushState = function(...args) {
                originalPushState.apply(window.history, args);
                setTimeout(trackPageChange, 100);
            };
            
            window.history.replaceState = function(...args) {
                originalReplaceState.apply(window.history, args);
                setTimeout(trackPageChange, 100);
            };
        }
        
        // Also track popstate events (back/forward)
        window.addEventListener('popstate', function() {
            setTimeout(trackPageChange, 100);
        });
        
        // Track hash changes
        window.addEventListener('hashchange', function() {
            setTimeout(trackPageChange, 100);
        });

        // Handle page visibility changes
        if (typeof document.visibilityState !== 'undefined') {
            document.addEventListener('visibilitychange', function() {
                if (document.visibilityState === 'visible') {
                    // Always reconnect when page becomes visible if not connected
                    if (!isConnected) {
                        debug('Page became visible, reconnecting...');
                        reconnectAttempts = 0; // Reset reconnect attempts
                        
                        // Clean up any stale websocket first
                        if (websocket && websocket.readyState !== WebSocket.OPEN && websocket.readyState !== WebSocket.CONNECTING) {
                            websocket.close();
                            websocket = null;
                        }
                        
                        // Only reconnect if we don't have an active connection attempt
                        if (!websocket) {
                            setTimeout(connect, 500);
                        }
                    }
                } else if (document.visibilityState === 'hidden') {
                    debug('Page became hidden');
                    // Keep connection alive but reduce heartbeat frequency
                }
            });
        }

        // Handle page unload
        window.addEventListener('beforeunload', function() {
            if (websocket && websocket.readyState === WebSocket.OPEN) {
                // Clean close to prevent reconnection attempts
                isConnected = false;
                stopHeartbeat();
                websocket.close(1000, 'Page unload');
                websocket = null;
            }
        });
        
        // Handle page reload/navigation
        window.addEventListener('pagehide', function() {
            if (websocket && websocket.readyState === WebSocket.OPEN) {
                // Clean close to prevent reconnection attempts
                isConnected = false;
                stopHeartbeat();
                websocket.close(1000, 'Page unload');
                websocket = null;
            }
        });

        // Handle online/offline events
        window.addEventListener('online', function() {
            debug('Connection restored');
            if (!isConnected && !websocket) {
                reconnectAttempts = 0; // Reset reconnect attempts
                setTimeout(connect, 1000);
            }
        });

        window.addEventListener('offline', function() {
            debug('Connection lost');
            handleDisconnection();
        });
    }

    /**
     * Public API (minimal exposure)
     */
    window.PayButtonAnalytics = {
        getSiteName: function() {
            return siteName;
        },
        getSessionId: function() {
            return sessionId;
        },
        isConnected: function() {
            return isConnected;
        },
        getCurrentPage: function() { // Phase 5
            return currentPagePath;
        },
        trackPageChange: trackPageChange, // Phase 5: Manual page tracking
        reconnect: function() {
            console.log('PayButton Analytics: Manual reconnect requested');
            reconnectAttempts = 0;
            isConnected = false;
            if (websocket) {
                websocket.close();
                websocket = null;
            }
            connect();
        },
        // Debug function to check status
        debug: function() {
            console.log('PayButton Analytics Debug:', {
                siteName: siteName,
                isConnected: isConnected,
                websocket: websocket ? websocket.readyState : 'null',
                sessionId: sessionId,
                reconnectAttempts: reconnectAttempts
            });
        }
    };

    // Auto-initialize when script loads
    console.log('PayButton Analytics: Script loaded, auto-initializing...');
    
    // Ensure initialization happens
    if (document.readyState === 'complete' || document.readyState === 'interactive') {
        // Document is ready, initialize immediately
        initialize();
    } else {
        // Wait for document to be ready
        document.addEventListener('DOMContentLoaded', initialize);
    }
    
    // Also try to initialize on load event as backup
    window.addEventListener('load', function() {
        if (!siteName) {
            console.log('PayButton Analytics: Reinitializing on load event');
            initialize();
        } else if (!isConnected) {
            console.log('PayButton Analytics: Not connected on load, checking connection state');
            
            // Clean up stale connections
            if (websocket && websocket.readyState !== WebSocket.OPEN && websocket.readyState !== WebSocket.CONNECTING) {
                console.log('PayButton Analytics: Cleaning up stale websocket');
                websocket.close();
                websocket = null;
            }
            
            // Attempt connection if none exists
            if (!websocket) {
                console.log('PayButton Analytics: Attempting fresh connection');
                reconnectAttempts = 0; // Reset attempts
                connect();
            }
        }
    });

})();