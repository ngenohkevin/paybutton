/**
 * RefreshManager - Centralized polling coordinator with CPU optimization
 *
 * Features:
 * - Visibility API: stops polling when tab is hidden
 * - Coordinated cleanup: prevents memory leaks
 * - Smart debouncing: prevents redundant updates
 * - Interval management: consistent polling across pages
 */

window.RefreshManager = (function() {
    const intervals = {};
    const debounceTimers = {};
    let isPageVisible = !document.hidden;

    // Track page visibility globally
    document.addEventListener('visibilitychange', function() {
        isPageVisible = !document.hidden;

        // Resume all registered intervals when page becomes visible
        if (isPageVisible) {
            Object.keys(intervals).forEach(name => {
                if (intervals[name] && intervals[name].callback) {
                    intervals[name].callback(); // Execute immediately
                }
            });
        }
    });

    return {
        /**
         * Register a polling interval
         * @param {string} name - Unique identifier for this interval
         * @param {function} callback - Function to call on each interval
         * @param {number} intervalMs - Interval in milliseconds (default: 60000)
         */
        register(name, callback, intervalMs = 60000) {
            // Clear existing interval if any
            if (intervals[name]) {
                clearInterval(intervals[name].timer);
            }

            // Store callback for visibility handling
            intervals[name] = {
                callback: callback,
                timer: setInterval(() => {
                    if (isPageVisible) {  // Only execute when page is visible
                        callback();
                    }
                }, intervalMs)
            };

            console.log(`RefreshManager: Registered "${name}" with ${intervalMs/1000}s interval`);
        },

        /**
         * Unregister and stop an interval
         * @param {string} name - Interval identifier to stop
         */
        unregister(name) {
            if (intervals[name]) {
                clearInterval(intervals[name].timer);
                delete intervals[name];
                console.log(`RefreshManager: Unregistered "${name}"`);
            }
        },

        /**
         * Debounced function execution
         * @param {string} name - Unique identifier for debounce
         * @param {function} callback - Function to call after delay
         * @param {number} delayMs - Delay in milliseconds (default: 500)
         */
        debounce(name, callback, delayMs = 500) {
            if (debounceTimers[name]) {
                clearTimeout(debounceTimers[name]);
            }

            debounceTimers[name] = setTimeout(() => {
                callback();
                delete debounceTimers[name];
            }, delayMs);
        },

        /**
         * Cleanup all intervals and timers
         */
        cleanup() {
            Object.keys(intervals).forEach(name => {
                clearInterval(intervals[name].timer);
            });
            Object.values(debounceTimers).forEach(clearTimeout);

            intervals = {};
            debounceTimers = {};
            console.log('RefreshManager: All intervals cleaned up');
        },

        /**
         * Check if page is currently visible
         */
        isVisible() {
            return isPageVisible;
        },

        /**
         * Get status of all registered intervals
         */
        getStatus() {
            return {
                activeIntervals: Object.keys(intervals).length,
                pendingDebounces: Object.keys(debounceTimers).length,
                isVisible: isPageVisible
            };
        }
    };
})();

// Cleanup on page unload
window.addEventListener('beforeunload', function() {
    RefreshManager.cleanup();
});

console.log('✅ RefreshManager initialized');
