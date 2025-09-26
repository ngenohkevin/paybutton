/**
 * PayButton Analytics SDK v2.0
 * Enhanced privacy-focused, real-time site analytics with Tor support
 * Improvements: Connection pooling, fallback tracking, performance optimizations
 */
(function() {
    'use strict';

    // Enhanced Configuration
    const CONFIG = {
        // Connection settings
        HEARTBEAT_INTERVAL: 30000, // 30 seconds (reduced frequency)
        BATCH_INTERVAL: 5000, // 5 seconds for batching events
        RECONNECT_DELAYS: [1000, 2000, 5000, 10000, 30000, 60000], // Smarter backoff
        MAX_RECONNECT_ATTEMPTS: 6,
        CONNECTION_TIMEOUT: 15000, // 15 seconds
        IDLE_TIMEOUT: 300000, // 5 minutes of inactivity before disconnect
        
        // Performance settings
        MAX_QUEUE_SIZE: 50, // Maximum events to queue
        COMPRESSION_THRESHOLD: 1024, // Compress messages larger than 1KB
        DEBOUNCE_DELAY: 500, // Debounce for rapid events
        
        // Fallback settings
        BEACON_ENDPOINT: '/analytics/beacon',
        FALLBACK_ENABLED: true,
        FALLBACK_RETRY_INTERVAL: 60000, // Try WebSocket again every minute
        
        // Advanced features
        TRACK_ENGAGEMENT: true, // Track time on page, scroll depth
        TRACK_PERFORMANCE: true, // Track page load times
        TRACK_ERRORS: false, // Disabled by default for privacy
        FINGERPRINT_RESISTANCE: true // Prevent fingerprinting
    };

    // Analytics state management
    class AnalyticsState {
        constructor() {
            this.websocket = null;
            this.reconnectAttempts = 0;
            this.heartbeatTimer = null;
            this.batchTimer = null;
            this.idleTimer = null;
            this.fallbackTimer = null;
            this.sessionId = this.generateSessionId();
            this.siteName = null;
            this.isConnected = false;
            this.useFallback = false;
            this.connectionStartTime = null;
            this.lastActivityTime = Date.now();
            
            // Analytics data
            this.currentPage = {
                path: null,
                title: null,
                referrer: null,
                entryTime: Date.now(),
                engagementTime: 0,
                scrollDepth: 0,
                interactions: 0
            };
            
            // Event queue for batching
            this.eventQueue = [];
            
            // Performance metrics
            this.metrics = {
                connectionAttempts: 0,
                successfulConnections: 0,
                failedConnections: 0,
                messagesSent: 0,
                messagesQueued: 0,
                fallbackUsed: 0
            };
            
            // Visibility tracking
            this.visibilityState = document.visibilityState || 'visible';
            this.hiddenTime = null;
            this.visibleTime = Date.now();
        }
        
        generateSessionId() {
            // More robust session ID generation
            const timestamp = Date.now();
            const random = Math.random().toString(36).substring(2, 15);
            const browserEntropy = this.getBrowserEntropy();
            return `${timestamp}_${random}_${browserEntropy}`;
        }
        
        getBrowserEntropy() {
            // Generate entropy without fingerprinting
            if (CONFIG.FINGERPRINT_RESISTANCE) {
                return Math.floor(Math.random() * 10000);
            }
            
            // Optional: Use more browser characteristics if fingerprinting is acceptable
            const entropy = [
                window.screen.width,
                window.screen.height,
                new Date().getTimezoneOffset(),
                navigator.hardwareConcurrency || 1
            ].reduce((acc, val) => acc + val, 0);
            
            return entropy % 10000;
        }
        
        updateActivity() {
            this.lastActivityTime = Date.now();
            this.resetIdleTimer();
        }
        
        resetIdleTimer() {
            if (this.idleTimer) {
                clearTimeout(this.idleTimer);
            }
            
            this.idleTimer = setTimeout(() => {
                if (this.isConnected && !this.useFallback) {
                    debug('Idle timeout reached, disconnecting to save resources');
                    this.disconnect(true); // Soft disconnect, can reconnect on activity
                }
            }, CONFIG.IDLE_TIMEOUT);
        }
        
        disconnect(soft = false) {
            this.isConnected = false;
            this.stopTimers();
            
            if (this.websocket) {
                if (this.websocket.readyState === WebSocket.OPEN) {
                    // Send any queued events before closing
                    this.flushQueue();
                    
                    // Send disconnect event
                    try {
                        this.websocket.send(JSON.stringify({
                            type: 'disconnect',
                            reason: soft ? 'idle' : 'close',
                            sessionId: this.sessionId,
                            timestamp: new Date().toISOString()
                        }));
                    } catch (e) {
                        // Ignore errors on disconnect
                    }
                }
                
                this.websocket.close(1000, soft ? 'Idle disconnect' : 'User disconnect');
                this.websocket = null;
            }
            
            if (!soft) {
                // Hard disconnect - clear session
                this.sessionId = null;
            }
        }
        
        stopTimers() {
            if (this.heartbeatTimer) {
                clearInterval(this.heartbeatTimer);
                this.heartbeatTimer = null;
            }
            
            if (this.batchTimer) {
                clearInterval(this.batchTimer);
                this.batchTimer = null;
            }
            
            if (this.idleTimer) {
                clearTimeout(this.idleTimer);
                this.idleTimer = null;
            }
            
            if (this.fallbackTimer) {
                clearTimeout(this.fallbackTimer);
                this.fallbackTimer = null;
            }
        }
        
        queueEvent(event) {
            // Add event to queue with timestamp
            event.timestamp = event.timestamp || new Date().toISOString();
            event.sessionId = this.sessionId;
            
            this.eventQueue.push(event);
            this.metrics.messagesQueued++;
            
            // Prevent queue overflow
            if (this.eventQueue.length > CONFIG.MAX_QUEUE_SIZE) {
                this.eventQueue.shift(); // Remove oldest event
            }
            
            // Update activity
            this.updateActivity();
        }
        
        flushQueue() {
            if (this.eventQueue.length === 0) return;
            
            const events = this.eventQueue.splice(0, CONFIG.MAX_QUEUE_SIZE);
            
            if (this.isConnected && this.websocket && this.websocket.readyState === WebSocket.OPEN) {
                // Send via WebSocket
                this.sendViaWebSocket(events);
            } else if (CONFIG.FALLBACK_ENABLED) {
                // Use fallback beacon
                this.sendViaBeacon(events);
            }
        }
        
        sendViaWebSocket(events) {
            try {
                const message = {
                    type: 'batch',
                    events: events,
                    sessionId: this.sessionId,
                    timestamp: new Date().toISOString()
                };
                
                const messageStr = JSON.stringify(message);
                
                // Compress large messages if supported
                if (messageStr.length > CONFIG.COMPRESSION_THRESHOLD && this.websocket.extensions.includes('permessage-deflate')) {
                    // Compression is handled by WebSocket extension
                }
                
                this.websocket.send(messageStr);
                this.metrics.messagesSent += events.length;
                
                debug(`Sent batch of ${events.length} events via WebSocket`);
            } catch (error) {
                debug('Failed to send via WebSocket:', error);
                // Re-queue events for retry
                this.eventQueue.unshift(...events);
            }
        }
        
        sendViaBeacon(events) {
            if (!navigator.sendBeacon) {
                debug('Beacon API not supported');
                return;
            }
            
            try {
                const payload = JSON.stringify({
                    site: this.siteName,
                    sessionId: this.sessionId,
                    events: events
                });
                
                const host = getAnalyticsHost();
                const protocol = window.location.protocol;
                const url = `${protocol}//${host}${CONFIG.BEACON_ENDPOINT}`;
                const success = navigator.sendBeacon(url, payload);
                
                if (success) {
                    this.metrics.fallbackUsed++;
                    debug(`Sent ${events.length} events via beacon fallback`);
                } else {
                    debug('Beacon send failed');
                    // Re-queue events
                    this.eventQueue.unshift(...events);
                }
            } catch (error) {
                debug('Beacon error:', error);
            }
        }
    }

    // Global state instance
    let state = null;

    /**
     * Enhanced site name extraction
     */
    function extractSiteName() {
        // Try multiple methods to get site name
        const methods = [
            // Method 1: URL parameter
            () => {
                const scripts = document.getElementsByTagName('script');
                for (let script of scripts) {
                    if (script.src && script.src.includes('analytics')) {
                        try {
                            const url = new URL(script.src);
                            const siteParam = url.searchParams.get('site');
                            if (siteParam) return siteParam.trim();
                        } catch (e) {}
                    }
                }
                return null;
            },
            
            // Method 2: Data attribute
            () => {
                const scripts = document.getElementsByTagName('script');
                for (let script of scripts) {
                    if (script.src && script.src.includes('analytics')) {
                        const siteAttr = script.getAttribute('data-site');
                        if (siteAttr) return siteAttr.trim();
                    }
                }
                return null;
            },
            
            // Method 3: Meta tag
            () => {
                const meta = document.querySelector('meta[name="paybutton-analytics-site"]');
                return meta ? meta.content.trim() : null;
            },
            
            // Method 4: Window variable
            () => window.PAYBUTTON_ANALYTICS_SITE || null,
            
            // Method 5: Cleaned hostname
            () => {
                const hostname = window.location.hostname
                    .replace(/^www\./, '')
                    .replace(/[^a-zA-Z0-9.-]/g, '_')
                    .substring(0, 50);
                return hostname || 'unknown';
            }
        ];
        
        for (let method of methods) {
            const siteName = method();
            if (siteName) {
                debug(`Site name detected: ${siteName}`);
                return siteName;
            }
        }
        
        return 'unknown';
    }

    /**
     * Get analytics host with fallback
     */
    function getAnalyticsHost() {
        // Try to get from script source
        const scripts = document.getElementsByTagName('script');
        for (let script of scripts) {
            if (script.src && script.src.includes('analytics')) {
                try {
                    const url = new URL(script.src);
                    return url.host;
                } catch (e) {}
            }
        }
        
        // Check for configured host
        if (window.PAYBUTTON_ANALYTICS_HOST) {
            return window.PAYBUTTON_ANALYTICS_HOST;
        }
        
        // Default to current host
        return window.location.host;
    }

    /**
     * Enhanced page tracking
     */
    function getPageInfo() {
        const path = window.location.pathname;
        const cleanPath = path
            .replace(/\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, '/[uuid]')
            .replace(/\/\d{6,}/g, '/[id]')
            .replace(/\/[A-Za-z0-9+\/=]{20,}/g, '/[token]')
            .substring(0, 200);
        
        return {
            path: cleanPath || '/',
            title: document.title ? document.title.substring(0, 100) : '',
            referrer: document.referrer ? new URL(document.referrer).hostname : '',
            url: window.location.href.substring(0, 500),
            timestamp: new Date().toISOString()
        };
    }

    /**
     * Track engagement metrics
     */
    function trackEngagement() {
        if (!CONFIG.TRACK_ENGAGEMENT || !state) return;
        
        // Track time on page
        const now = Date.now();
        if (state.visibilityState === 'visible') {
            state.currentPage.engagementTime += now - state.visibleTime;
        }
        state.visibleTime = now;
        
        // Track scroll depth
        const scrollHeight = document.documentElement.scrollHeight - window.innerHeight;
        if (scrollHeight > 0) {
            const scrollPercent = (window.scrollY / scrollHeight) * 100;
            state.currentPage.scrollDepth = Math.max(state.currentPage.scrollDepth, Math.round(scrollPercent));
        }
        
        // Track interactions
        state.currentPage.interactions++;
        
        // Queue engagement update
        state.queueEvent({
            type: 'engagement',
            page: getPageInfo(),
            metrics: {
                timeOnPage: Math.round(state.currentPage.engagementTime / 1000), // seconds
                scrollDepth: state.currentPage.scrollDepth,
                interactions: state.currentPage.interactions
            }
        });
    }

    /**
     * Track performance metrics
     */
    function trackPerformance() {
        if (!CONFIG.TRACK_PERFORMANCE || !state || !window.performance) return;
        
        const perfData = performance.getEntriesByType('navigation')[0] || {};
        
        const metrics = {
            loadTime: perfData.loadEventEnd - perfData.fetchStart,
            domContentLoaded: perfData.domContentLoadedEventEnd - perfData.fetchStart,
            firstPaint: 0,
            firstContentfulPaint: 0
        };
        
        // Get paint timings
        const paintEntries = performance.getEntriesByType('paint');
        paintEntries.forEach(entry => {
            if (entry.name === 'first-paint') {
                metrics.firstPaint = Math.round(entry.startTime);
            } else if (entry.name === 'first-contentful-paint') {
                metrics.firstContentfulPaint = Math.round(entry.startTime);
            }
        });
        
        // Only send if we have meaningful data
        if (metrics.loadTime > 0) {
            state.queueEvent({
                type: 'performance',
                page: getPageInfo(),
                metrics: metrics
            });
        }
    }

    /**
     * Enhanced debug logging
     */
    function debug(...args) {
        if (window.PAYBUTTON_ANALYTICS_DEBUG || window.location.hash === '#debug-analytics') {
            console.log('[PayButton Analytics v2]', ...args);
        }
    }

    /**
     * WebSocket connection with improvements
     */
    function connect() {
        if (!state || !state.siteName) {
            debug('Cannot connect: no state or site name');
            return;
        }
        
        // Check if already connected
        if (state.websocket && state.websocket.readyState === WebSocket.OPEN) {
            debug('Already connected');
            return;
        }
        
        // Update metrics
        state.metrics.connectionAttempts++;
        state.connectionStartTime = Date.now();
        
        // Clean up existing connection
        if (state.websocket) {
            state.websocket.close();
            state.websocket = null;
        }
        
        debug(`Connecting to analytics for site: ${state.siteName}`);
        
        try {
            // Build WebSocket URL with parameters
            // Use wss: for HTTPS hosts, ws: for HTTP/local development
            const host = getAnalyticsHost();
            const protocol = (window.location.protocol === 'https:' || host.includes('https://')) ? 'wss:' : 'ws:';
            const params = new URLSearchParams({
                path: state.currentPage.path,
                tz: new Date().getTimezoneOffset(),
                v: '2.0' // Version for compatibility
            });
            
            const wsUrl = `${protocol}//${host}/ws/analytics/v2/${state.siteName}?${params}`;
            
            // Create WebSocket with options
            state.websocket = new WebSocket(wsUrl);
            
            // Connection timeout
            const connectionTimer = setTimeout(() => {
                if (state.websocket && state.websocket.readyState === WebSocket.CONNECTING) {
                    debug('Connection timeout, trying fallback');
                    state.websocket.close();
                    handleConnectionFailure();
                }
            }, CONFIG.CONNECTION_TIMEOUT);
            
            // WebSocket event handlers
            state.websocket.onopen = function(event) {
                clearTimeout(connectionTimer);
                const connectionTime = Date.now() - state.connectionStartTime;
                
                state.isConnected = true;
                state.useFallback = false;
                state.reconnectAttempts = 0;
                state.metrics.successfulConnections++;
                
                debug(`Connected in ${connectionTime}ms`);
                
                // Start timers
                startHeartbeat();
                startBatchTimer();
                state.resetIdleTimer();
                
                // Send initial page view
                state.queueEvent({
                    type: 'pageview',
                    page: getPageInfo()
                });
                
                // Track performance after connection
                setTimeout(trackPerformance, 100);
                
                // Flush any queued events
                state.flushQueue();
            };
            
            state.websocket.onmessage = function(event) {
                try {
                    const data = JSON.parse(event.data);
                    handleServerMessage(data);
                } catch (error) {
                    debug('Message parse error:', error);
                }
            };
            
            state.websocket.onclose = function(event) {
                clearTimeout(connectionTimer);
                debug(`WebSocket closed: code=${event.code}, reason=${event.reason}`);
                handleDisconnection(event.code);
            };
            
            state.websocket.onerror = function(error) {
                clearTimeout(connectionTimer);
                debug('WebSocket error:', error);
                state.metrics.failedConnections++;
            };
            
        } catch (error) {
            debug('Connection error:', error);
            handleConnectionFailure();
        }
    }

    /**
     * Handle server messages
     */
    function handleServerMessage(data) {
        switch (data.type) {
            case 'config':
                // Server can send configuration updates
                if (data.heartbeatInterval) {
                    CONFIG.HEARTBEAT_INTERVAL = data.heartbeatInterval;
                    restartHeartbeat();
                }
                break;
                
            case 'ack':
                // Acknowledgment of received events
                debug(`Server acknowledged ${data.count} events`);
                break;
                
            case 'error':
                debug('Server error:', data.message);
                break;
                
            default:
                debug('Unknown message type:', data.type);
        }
    }

    /**
     * Start heartbeat with jitter
     */
    function startHeartbeat() {
        if (state.heartbeatTimer) {
            clearInterval(state.heartbeatTimer);
        }
        
        // Add jitter to prevent thundering herd
        const jitter = Math.random() * 5000; // 0-5 seconds
        
        state.heartbeatTimer = setInterval(() => {
            if (state.websocket && state.websocket.readyState === WebSocket.OPEN) {
                try {
                    state.websocket.send(JSON.stringify({
                        type: 'heartbeat',
                        sessionId: state.sessionId,
                        timestamp: new Date().toISOString(),
                        metrics: {
                            queueSize: state.eventQueue.length,
                            uptime: Date.now() - state.connectionStartTime
                        }
                    }));
                } catch (error) {
                    debug('Heartbeat failed:', error);
                    handleDisconnection();
                }
            }
        }, CONFIG.HEARTBEAT_INTERVAL + jitter);
    }

    /**
     * Start batch timer for sending queued events
     */
    function startBatchTimer() {
        if (state.batchTimer) {
            clearInterval(state.batchTimer);
        }
        
        state.batchTimer = setInterval(() => {
            if (state.eventQueue.length > 0) {
                state.flushQueue();
            }
        }, CONFIG.BATCH_INTERVAL);
    }

    /**
     * Handle disconnection with smart reconnection
     */
    function handleDisconnection(code) {
        state.isConnected = false;
        state.stopTimers();
        
        if (state.websocket) {
            state.websocket = null;
        }
        
        // Don't reconnect if page is unloading
        if (document.readyState === 'unloading' || state.visibilityState === 'hidden') {
            return;
        }
        
        // Check if we should use fallback
        if (CONFIG.FALLBACK_ENABLED && state.reconnectAttempts >= 2) {
            debug('Switching to fallback mode');
            state.useFallback = true;
            startFallbackMode();
        } else {
            // Attempt reconnection with backoff
            scheduleReconnection();
        }
    }

    /**
     * Handle connection failure
     */
    function handleConnectionFailure() {
        state.metrics.failedConnections++;
        
        if (CONFIG.FALLBACK_ENABLED) {
            debug('Connection failed, using fallback');
            state.useFallback = true;
            startFallbackMode();
        } else {
            scheduleReconnection();
        }
    }

    /**
     * Schedule reconnection with exponential backoff
     */
    function scheduleReconnection() {
        if (state.reconnectAttempts >= CONFIG.MAX_RECONNECT_ATTEMPTS) {
            debug('Max reconnection attempts reached');
            if (CONFIG.FALLBACK_ENABLED) {
                startFallbackMode();
            }
            return;
        }
        
        const delay = CONFIG.RECONNECT_DELAYS[Math.min(state.reconnectAttempts, CONFIG.RECONNECT_DELAYS.length - 1)];
        debug(`Reconnecting in ${delay}ms (attempt ${state.reconnectAttempts + 1})`);
        
        setTimeout(() => {
            state.reconnectAttempts++;
            connect();
        }, delay);
    }

    /**
     * Start fallback mode using beacon API
     */
    function startFallbackMode() {
        debug('Fallback mode activated');
        
        // Send queued events via beacon
        state.flushQueue();
        
        // Start batch timer for fallback
        if (state.batchTimer) {
            clearInterval(state.batchTimer);
        }
        
        state.batchTimer = setInterval(() => {
            if (state.eventQueue.length > 0) {
                state.flushQueue();
            }
        }, CONFIG.BATCH_INTERVAL * 2); // Slower rate for fallback
        
        // Periodically try to reconnect WebSocket
        state.fallbackTimer = setTimeout(() => {
            debug('Attempting to restore WebSocket connection');
            state.reconnectAttempts = 0;
            state.useFallback = false;
            connect();
        }, CONFIG.FALLBACK_RETRY_INTERVAL);
    }

    /**
     * Restart heartbeat (used when config changes)
     */
    function restartHeartbeat() {
        if (state.heartbeatTimer) {
            clearInterval(state.heartbeatTimer);
            startHeartbeat();
        }
    }

    /**
     * Track page changes for SPAs
     */
    function trackPageChange() {
        if (!state) return;
        
        const newPageInfo = getPageInfo();
        
        // Check if page actually changed
        if (newPageInfo.path !== state.currentPage.path) {
            // Send page exit event for previous page
            if (state.currentPage.path) {
                trackEngagement();
            }
            
            // Update current page
            state.currentPage = {
                path: newPageInfo.path,
                title: newPageInfo.title,
                referrer: state.currentPage.path, // Previous page becomes referrer
                entryTime: Date.now(),
                engagementTime: 0,
                scrollDepth: 0,
                interactions: 0
            };
            
            // Queue page view event
            state.queueEvent({
                type: 'pageview',
                page: newPageInfo
            });
            
            // Update activity
            state.updateActivity();
            
            // Reconnect if needed (for significant changes)
            if (!state.isConnected && !state.useFallback) {
                connect();
            }
        }
    }

    /**
     * Initialize event listeners
     */
    function initializeEventListeners() {
        // Page visibility changes
        document.addEventListener('visibilitychange', function() {
            const wasVisible = state.visibilityState === 'visible';
            state.visibilityState = document.visibilityState;
            
            if (state.visibilityState === 'visible') {
                debug('Page became visible');
                state.visibleTime = Date.now();
                
                // Track time hidden
                if (state.hiddenTime) {
                    const hiddenDuration = Date.now() - state.hiddenTime;
                    debug(`Page was hidden for ${hiddenDuration}ms`);
                }
                
                // Reconnect if disconnected and not in fallback
                if (!state.isConnected && !state.useFallback) {
                    state.reconnectAttempts = 0;
                    setTimeout(connect, 1000);
                }
            } else {
                debug('Page became hidden');
                state.hiddenTime = Date.now();
                
                // Track engagement before hiding
                if (wasVisible) {
                    trackEngagement();
                }
                
                // Flush events before page hides
                state.flushQueue();
            }
        });
        
        // SPA navigation tracking
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
        
        window.addEventListener('popstate', () => setTimeout(trackPageChange, 100));
        window.addEventListener('hashchange', () => setTimeout(trackPageChange, 100));
        
        // User interaction tracking
        if (CONFIG.TRACK_ENGAGEMENT) {
            // Debounced scroll tracking
            let scrollTimer = null;
            window.addEventListener('scroll', function() {
                if (scrollTimer) clearTimeout(scrollTimer);
                scrollTimer = setTimeout(() => {
                    if (state) {
                        const scrollHeight = document.documentElement.scrollHeight - window.innerHeight;
                        if (scrollHeight > 0) {
                            const scrollPercent = (window.scrollY / scrollHeight) * 100;
                            state.currentPage.scrollDepth = Math.max(state.currentPage.scrollDepth, Math.round(scrollPercent));
                        }
                    }
                }, CONFIG.DEBOUNCE_DELAY);
            }, { passive: true });
            
            // Click tracking (privacy-safe)
            document.addEventListener('click', function() {
                if (state) {
                    state.currentPage.interactions++;
                    state.updateActivity();
                }
            }, { passive: true });
        }
        
        // Network status
        window.addEventListener('online', function() {
            debug('Network online');
            if (!state.isConnected && !state.useFallback) {
                state.reconnectAttempts = 0;
                setTimeout(connect, 1000);
            }
        });
        
        window.addEventListener('offline', function() {
            debug('Network offline');
            if (state.isConnected) {
                state.disconnect(true);
            }
        });
        
        // Page unload
        window.addEventListener('beforeunload', function() {
            if (state) {
                // Track final engagement
                trackEngagement();
                
                // Send any remaining events
                state.flushQueue();
                
                // Clean disconnect
                if (state.websocket) {
                    state.disconnect(false);
                }
            }
        });
        
        // Alternative unload event
        window.addEventListener('pagehide', function(event) {
            if (state && event.persisted === false) {
                state.flushQueue();
                state.disconnect(false);
            }
        });
    }

    /**
     * Initialize analytics
     */
    function initialize() {
        // Check if already initialized
        if (state) {
            debug('Already initialized');
            return;
        }
        
        debug('Initializing PayButton Analytics v2');
        
        // Create state
        state = new AnalyticsState();
        
        // Extract site name
        state.siteName = extractSiteName();
        
        if (!state.siteName || state.siteName === 'unknown') {
            console.warn('PayButton Analytics: Unable to determine site name');
        }
        
        // Get initial page info
        const pageInfo = getPageInfo();
        state.currentPage.path = pageInfo.path;
        state.currentPage.title = pageInfo.title;
        state.currentPage.referrer = pageInfo.referrer;
        
        // Initialize event listeners
        initializeEventListeners();
        
        // Connect when ready
        if (document.readyState === 'complete') {
            connect();
        } else {
            window.addEventListener('load', connect);
        }
        
        debug(`Initialized for site: ${state.siteName}`);
    }

    /**
     * Public API
     */
    window.PayButtonAnalytics = {
        // Core functions
        initialize: initialize,
        connect: () => state && connect(),
        disconnect: () => state && state.disconnect(false),
        
        // State getters
        getSiteName: () => state ? state.siteName : null,
        getSessionId: () => state ? state.sessionId : null,
        isConnected: () => state ? state.isConnected : false,
        isUsingFallback: () => state ? state.useFallback : false,
        
        // Analytics functions
        trackEvent: (eventName, data) => {
            if (state) {
                state.queueEvent({
                    type: 'custom',
                    name: eventName,
                    data: data
                });
            }
        },
        
        trackPageView: (path) => {
            if (state) {
                state.currentPage.path = path || window.location.pathname;
                trackPageChange();
            }
        },
        
        // Metrics and debugging
        getMetrics: () => state ? state.metrics : {},
        getQueueSize: () => state ? state.eventQueue.length : 0,
        
        debug: () => {
            if (!state) {
                console.log('PayButton Analytics: Not initialized');
                return;
            }
            
            console.table({
                'Site Name': state.siteName,
                'Session ID': state.sessionId,
                'Connected': state.isConnected,
                'Using Fallback': state.useFallback,
                'Queue Size': state.eventQueue.length,
                'Current Page': state.currentPage.path,
                'Engagement Time': `${Math.round(state.currentPage.engagementTime / 1000)}s`,
                'Scroll Depth': `${state.currentPage.scrollDepth}%`,
                'Interactions': state.currentPage.interactions
            });
            
            console.table(state.metrics);
        },
        
        // Configuration
        configure: (options) => {
            Object.assign(CONFIG, options);
            debug('Configuration updated:', options);
        },
        
        // Version
        version: '2.0.0'
    };
    
    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initialize);
    } else {
        initialize();
    }
    
    // Expose debug function globally in development
    if (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1') {
        window.pba = window.PayButtonAnalytics;
    }

})();