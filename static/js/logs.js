/**
 * Enhanced Log Viewer JavaScript
 * Provides advanced functionality for the admin logs interface
 */

class EnhancedLogViewer {
    constructor() {
        this.eventSource = null;
        this.isStreaming = false;
        this.autoScroll = true;
        this.logCount = 0;
        this.maxLogs = 1000;
        this.stats = { error: 0, warn: 0, info: 0, debug: 0 };
        this.filters = { level: 'all', component: 'all', search: '' };
        this.logBuffer = [];
        
        this.initializeElements();
        this.bindEvents();
        this.setupKeyboardShortcuts();
        this.initializeTooltips();
    }
    
    initializeElements() {
        this.logContainer = document.getElementById('log-container');
        this.logContent = document.getElementById('log-content');
        this.streamIndicator = document.getElementById('stream-indicator');
        this.streamText = document.getElementById('stream-text');
        this.toggleStreamBtn = document.getElementById('toggle-stream');
        this.autoScrollBtn = document.getElementById('auto-scroll-toggle');
        this.logCountSpan = document.getElementById('log-count');
        this.connectionIndicator = document.getElementById('connection-indicator');
        
        // Filter elements
        this.levelFilter = document.getElementById('level-filter');
        this.componentFilter = document.getElementById('component-filter');
        this.searchFilter = document.getElementById('search-filter');
        this.applyFiltersBtn = document.getElementById('apply-filters');
        this.resetFiltersBtn = document.getElementById('reset-filters');
        
        // Action buttons
        this.downloadLogsBtn = document.getElementById('download-logs');
        this.clearLogsBtn = document.getElementById('clear-logs');
        this.fullscreenBtn = document.getElementById('fullscreen-toggle');
        
        // Stat elements
        this.statElements = {
            error: document.getElementById('error-count'),
            warning: document.getElementById('warning-count'),
            info: document.getElementById('info-count'),
            debug: document.getElementById('debug-count')
        };
    }
    
    bindEvents() {
        this.toggleStreamBtn?.addEventListener('click', () => this.toggleStream());
        this.autoScrollBtn?.addEventListener('click', () => this.toggleAutoScroll());
        this.applyFiltersBtn?.addEventListener('click', () => this.applyFilters());
        this.resetFiltersBtn?.addEventListener('click', () => this.resetFilters());
        this.downloadLogsBtn?.addEventListener('click', () => this.downloadLogs());
        this.clearLogsBtn?.addEventListener('click', () => this.clearLogs());
        this.fullscreenBtn?.addEventListener('click', () => this.toggleFullscreen());
        
        // Auto-apply filters on enter key
        this.searchFilter?.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                this.applyFilters();
            }
        });
        
        // Real-time search filtering
        this.searchFilter?.addEventListener('input', this.debounce(() => {
            this.applyClientSideFilters();
        }, 300));
        
        // Level and component filter changes
        this.levelFilter?.addEventListener('change', () => this.applyClientSideFilters());
        this.componentFilter?.addEventListener('change', () => this.applyClientSideFilters());
        
        // Scroll event for auto-scroll detection
        this.logContainer?.addEventListener('scroll', () => {
            if (this.autoScroll) {
                const { scrollTop, scrollHeight, clientHeight } = this.logContainer;
                if (scrollTop + clientHeight < scrollHeight - 50) {
                    this.setAutoScroll(false);
                }
            }
        });
        
        // Visibility change - pause/resume when tab is hidden/visible
        document.addEventListener('visibilitychange', () => {
            if (document.hidden && this.isStreaming) {
                this.pauseStream();
            } else if (!document.hidden && this.streamPaused) {
                this.resumeStream();
            }
        });
    }
    
    setupKeyboardShortcuts() {
        document.addEventListener('keydown', (e) => {
            // Only handle shortcuts when not typing in input fields
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
                return;
            }
            
            switch (e.key.toLowerCase()) {
                case 's':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.toggleStream();
                    }
                    break;
                case 'c':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.clearLogs();
                    }
                    break;
                case 'd':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.downloadLogs();
                    }
                    break;
                case 'f':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.searchFilter?.focus();
                    }
                    break;
                case 'r':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.resetFilters();
                    }
                    break;
                case ' ':
                    e.preventDefault();
                    this.toggleAutoScroll();
                    break;
                case 'escape':
                    this.searchFilter?.blur();
                    break;
            }
        });
    }
    
    initializeTooltips() {
        const elements = [
            { el: this.toggleStreamBtn, text: 'Ctrl+S to toggle stream' },
            { el: this.clearLogsBtn, text: 'Ctrl+C to clear logs' },
            { el: this.downloadLogsBtn, text: 'Ctrl+D to download logs' },
            { el: this.searchFilter, text: 'Ctrl+F to focus search' },
            { el: this.resetFiltersBtn, text: 'Ctrl+R to reset filters' },
            { el: this.autoScrollBtn, text: 'Space to toggle auto-scroll' }
        ];
        
        elements.forEach(({ el, text }) => {
            if (el) {
                el.title = text;
            }
        });
    }
    
    toggleStream() {
        if (this.isStreaming) {
            this.stopStream();
        } else {
            this.startStream();
        }
    }
    
    startStream() {
        this.updateFilters();
        const url = this.buildStreamUrl();
        
        this.eventSource = new EventSource(url);
        this.showConnectionIndicator();
        
        this.eventSource.onopen = () => {
            this.isStreaming = true;
            this.updateStreamStatus(true);
            this.hideConnectionIndicator();
            this.showNotification('success', '🔗 Connected to log stream');
        };
        
        this.eventSource.onmessage = (event) => {
            this.addLogEntry(event.data);
        };
        
        this.eventSource.onerror = (error) => {
            console.error('EventSource failed:', error);
            this.stopStream();
            this.showNotification('error', '❌ Log stream disconnected');
        };
    }
    
    stopStream() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
        }
        
        this.isStreaming = false;
        this.streamPaused = false;
        this.updateStreamStatus(false);
        this.hideConnectionIndicator();
    }
    
    pauseStream() {
        if (this.eventSource && this.isStreaming) {
            this.streamPaused = true;
            this.showNotification('info', '⏸️ Stream paused (tab hidden)');
        }
    }
    
    resumeStream() {
        if (this.streamPaused && this.isStreaming) {
            this.streamPaused = false;
            this.showNotification('info', '▶️ Stream resumed');
        }
    }
    
    updateFilters() {
        this.filters = {
            level: this.levelFilter?.value || 'all',
            component: this.componentFilter?.value || 'all',
            search: this.searchFilter?.value || ''
        };
    }
    
    buildStreamUrl() {
        const { level, component, search } = this.filters;
        return `/admin/api/logs/stream?level=${level}&component=${component}&search=${encodeURIComponent(search)}`;
    }
    
    updateStreamStatus(connected) {
        if (!this.streamIndicator || !this.streamText || !this.toggleStreamBtn) return;
        
        if (connected) {
            this.streamIndicator.className = 'connection-dot connected';
            this.streamText.textContent = 'Connected';
            this.toggleStreamBtn.innerHTML = '<i class="fas fa-stop mr-1"></i>Stop Stream';
            this.toggleStreamBtn.className = 'btn-log-action btn-log-danger active';
        } else {
            this.streamIndicator.className = 'connection-dot disconnected';
            this.streamText.textContent = 'Disconnected';
            this.toggleStreamBtn.innerHTML = '<i class="fas fa-play mr-1"></i>Start Stream';
            this.toggleStreamBtn.className = 'btn-log-action';
        }
    }
    
    addLogEntry(logData) {
        try {
            const entry = this.parseLogEntry(logData);
            if (!entry) return;
            
            this.logBuffer.push(entry);
            
            if (this.passesClientFilters(entry)) {
                const logElement = this.createLogElement(entry);
                this.appendLogElement(logElement);
                
                this.updateStats(entry.level);
                this.manageLogLimit();
                
                if (this.autoScroll) {
                    this.scrollToBottom();
                }
            }
            
            this.updateLogCount();
        } catch (error) {
            console.error('Error adding log entry:', error);
        }
    }
    
    parseLogEntry(logData) {
        // Enhanced parsing with better regex and fallbacks
        const patterns = [
            // Standard format: "2006-01-02 15:04:05 [LEVEL] [COMPONENT] message"
            /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+\[([A-Z]+)\]\s+\[([A-Z\s]+)\]\s+(.+)$/,
            // Alternative format without brackets
            /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+([A-Z]+)\s+([A-Z\s]+)\s+(.+)$/,
            // Simple timestamp + message
            /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+(.+)$/
        ];
        
        for (const pattern of patterns) {
            const match = logData.match(pattern);
            if (match) {
                if (match.length === 5) {
                    return {
                        timestamp: this.formatTimestamp(match[1]),
                        level: match[2].toUpperCase(),
                        component: match[3].trim().toUpperCase(),
                        message: match[4],
                        raw: logData
                    };
                } else if (match.length === 3) {
                    return {
                        timestamp: this.formatTimestamp(match[1]),
                        level: 'INFO',
                        component: 'SYSTEM',
                        message: match[2],
                        raw: logData
                    };
                }
            }
        }
        
        // Fallback for unstructured messages
        return {
            timestamp: this.formatTimestamp(new Date().toISOString()),
            level: 'INFO',
            component: 'SYSTEM',
            message: logData,
            raw: logData
        };
    }
    
    formatTimestamp(timestamp) {
        try {
            const date = new Date(timestamp);
            return date.toLocaleString();
        } catch {
            return timestamp;
        }
    }
    
    passesClientFilters(entry) {
        const { level, component, search } = this.filters;
        
        // Level filter
        if (level !== 'all' && entry.level.toLowerCase() !== level.toLowerCase()) {
            return false;
        }
        
        // Component filter
        if (component !== 'all' && !entry.component.toLowerCase().includes(component.toLowerCase())) {
            return false;
        }
        
        // Search filter
        if (search && !entry.message.toLowerCase().includes(search.toLowerCase())) {
            return false;
        }
        
        return true;
    }
    
    createLogElement(entry) {
        const logElement = document.createElement('div');
        logElement.className = `log-entry new level-${entry.level.toLowerCase()}`;
        logElement.setAttribute('data-level', entry.level);
        logElement.setAttribute('data-component', entry.component);
        logElement.setAttribute('data-timestamp', entry.timestamp);
        
        const levelColor = this.getLevelColor(entry.level);
        
        logElement.innerHTML = `
            <div class="flex items-start space-x-3 text-sm">
                <span class="log-timestamp text-gray-400 font-mono text-xs whitespace-nowrap">
                    ${entry.timestamp}
                </span>
                <span class="log-level px-2 py-0.5 rounded text-xs font-bold" style="background-color: ${levelColor}; color: white;">
                    ${entry.level}
                </span>
                <span class="log-component text-green-400 font-semibold text-xs">
                    [${entry.component}]
                </span>
                <span class="log-message flex-1 break-words">
                    ${this.highlightSearchTerm(entry.message)}
                </span>
            </div>
        `;
        
        // Remove 'new' class after animation
        setTimeout(() => {
            logElement.classList.remove('new');
        }, 300);
        
        return logElement;
    }
    
    getLevelColor(level) {
        const colors = {
            'ERROR': '#ff6b6b',
            'WARN': '#ffd93d',
            'INFO': '#74c0fc',
            'DEBUG': '#9775fa'
        };
        return colors[level.toUpperCase()] || '#868e96';
    }
    
    highlightSearchTerm(message) {
        if (!this.filters.search) return message;
        
        const regex = new RegExp(`(${this.escapeRegex(this.filters.search)})`, 'gi');
        return message.replace(regex, '<mark class="bg-yellow-200 dark:bg-yellow-600">$1</mark>');
    }
    
    escapeRegex(string) {
        return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }
    
    appendLogElement(element) {
        if (this.logContent) {
            this.logContent.appendChild(element);
            this.logCount++;
        }
    }
    
    updateStats(level) {
        const levelLower = level.toLowerCase();
        if (this.stats.hasOwnProperty(levelLower)) {
            this.stats[levelLower]++;
        }
        
        Object.entries(this.statElements).forEach(([key, element]) => {
            if (element) {
                element.textContent = this.stats[key] || 0;
            }
        });
    }
    
    updateLogCount() {
        if (this.logCountSpan) {
            this.logCountSpan.textContent = `${this.logCount} entries`;
        }
    }
    
    manageLogLimit() {
        if (this.logCount > this.maxLogs) {
            const entriesToRemove = this.logCount - this.maxLogs;
            const entries = this.logContent.querySelectorAll('.log-entry');
            
            for (let i = 0; i < entriesToRemove; i++) {
                if (entries[i]) {
                    entries[i].remove();
                    this.logCount--;
                }
            }
        }
    }
    
    toggleAutoScroll() {
        this.setAutoScroll(!this.autoScroll);
    }
    
    setAutoScroll(enabled) {
        this.autoScroll = enabled;
        
        if (this.autoScrollBtn) {
            if (enabled) {
                this.autoScrollBtn.innerHTML = '<i class="fas fa-arrow-down mr-1"></i>Auto Scroll';
                this.autoScrollBtn.className = 'btn-log-action active';
                this.scrollToBottom();
            } else {
                this.autoScrollBtn.innerHTML = '<i class="fas fa-pause mr-1"></i>Paused';
                this.autoScrollBtn.className = 'btn-log-action';
            }
        }
    }
    
    scrollToBottom() {
        if (this.logContainer) {
            this.logContainer.scrollTop = this.logContainer.scrollHeight;
        }
    }
    
    applyFilters() {
        if (this.isStreaming) {
            this.stopStream();
            setTimeout(() => this.startStream(), 100);
        } else {
            this.applyClientSideFilters();
        }
    }
    
    applyClientSideFilters() {
        this.updateFilters();
        const entries = this.logContent.querySelectorAll('.log-entry');
        
        entries.forEach(entry => {
            const level = entry.getAttribute('data-level');
            const component = entry.getAttribute('data-component');
            const message = entry.textContent || '';
            
            const fakeEntry = { level, component, message };
            const visible = this.passesClientFilters(fakeEntry);
            
            entry.style.display = visible ? 'block' : 'none';
        });
    }
    
    resetFilters() {
        if (this.levelFilter) this.levelFilter.value = 'all';
        if (this.componentFilter) this.componentFilter.value = 'all';
        if (this.searchFilter) this.searchFilter.value = '';
        
        this.applyFilters();
    }
    
    downloadLogs() {
        this.updateFilters();
        const { level, component, search } = this.filters;
        const url = `/admin/api/logs/download?level=${level}&component=${component}&search=${encodeURIComponent(search)}`;
        
        // Create temporary link for download
        const link = document.createElement('a');
        link.href = url;
        link.style.display = 'none';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        
        this.showNotification('success', '💾 Log download initiated');
    }
    
    clearLogs() {
        if (this.logContent) {
            this.logContent.innerHTML = `
                <div class="text-gray-500 italic p-4">
                    <i class="fas fa-info-circle mr-2"></i>
                    Logs cleared from display. Use "Start Stream" to begin viewing new logs...
                </div>
            `;
        }
        
        this.logCount = 0;
        this.stats = { error: 0, warn: 0, info: 0, debug: 0 };
        this.logBuffer = [];
        
        this.updateLogCount();
        Object.entries(this.statElements).forEach(([key, element]) => {
            if (element) element.textContent = '0';
        });
        
        this.showNotification('info', '🧹 Log display cleared');
    }
    
    showConnectionIndicator() {
        if (this.connectionIndicator) {
            this.connectionIndicator.classList.remove('hidden');
            this.streamIndicator.className = 'connection-dot connecting';
        }
    }
    
    hideConnectionIndicator() {
        if (this.connectionIndicator) {
            this.connectionIndicator.classList.add('hidden');
        }
    }
    
    showNotification(type, message) {
        // Use global notification function if available
        if (typeof showNotification === 'function') {
            showNotification(type, message);
        } else {
            // Fallback to console
            console.log(`${type.toUpperCase()}: ${message}`);
            
            // Create simple toast notification
            this.createToastNotification(type, message);
        }
    }
    
    createToastNotification(type, message) {
        const toast = document.createElement('div');
        toast.className = `
            fixed top-4 right-4 z-50 px-4 py-2 rounded-lg shadow-lg text-white text-sm
            ${type === 'success' ? 'bg-green-500' : 
              type === 'error' ? 'bg-red-500' : 
              type === 'info' ? 'bg-blue-500' : 'bg-gray-500'}
        `;
        toast.textContent = message;
        
        document.body.appendChild(toast);
        
        // Animate in
        setTimeout(() => toast.style.transform = 'translateX(0)', 10);
        
        // Remove after 3 seconds
        setTimeout(() => {
            toast.style.transform = 'translateX(100%)';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }
    
    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }
    
    // Utility method to export logs as JSON
    exportLogsAsJSON() {
        const exportData = {
            timestamp: new Date().toISOString(),
            filters: this.filters,
            stats: this.stats,
            logs: this.logBuffer.slice(-500), // Export last 500 entries
            metadata: {
                totalLogs: this.logCount,
                isStreaming: this.isStreaming,
                autoScroll: this.autoScroll
            }
        };
        
        const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `paybutton-logs-${new Date().toISOString().slice(0, 19)}.json`;
        link.click();
        URL.revokeObjectURL(url);
    }
    
    toggleFullscreen() {
        // Find the card containing the log container using a more compatible approach
        const logContainer = document.getElementById('log-container');
        let logsCard = logContainer ? logContainer.closest('.card') : null;
        
        // Fallback: find any card in the logs page if the first approach fails
        if (!logsCard) {
            const allCards = document.querySelectorAll('.card');
            for (let card of allCards) {
                if (card.querySelector('#log-container')) {
                    logsCard = card;
                    break;
                }
            }
        }
        
        const isFullscreen = logsCard && logsCard.classList.contains('logs-expanded');
        
        // Debug logging
        console.log('Toggle fullscreen:', { logsCard, isFullscreen, logContainer });
        
        if (isFullscreen) {
            this.exitFullscreen(logsCard);
        } else {
            this.enterFullscreen(logsCard);
        }
    }
    
    enterFullscreen(logsCard) {
        console.log('Enter fullscreen called with:', logsCard);
        if (!logsCard) {
            console.error('No logsCard found, cannot enter fullscreen');
            return;
        }
        
        // Get navigation and content boundaries for page-only fullscreen
        const nav = document.querySelector('nav');
        const sidebar = document.querySelector('.sidebar');
        
        // Calculate available space (page content area only)
        let topOffset = 0;
        let leftOffset = 0;
        let width = window.innerWidth;
        let height = window.innerHeight;
        
        if (nav && window.getComputedStyle(nav).display !== 'none') {
            const navRect = nav.getBoundingClientRect();
            topOffset = navRect.height;
            height -= navRect.height;
        }
        
        if (sidebar && window.getComputedStyle(sidebar).display !== 'none') {
            const sidebarRect = sidebar.getBoundingClientRect();
            leftOffset = sidebarRect.width;
            width -= sidebarRect.width;
        }
        
        // Apply page-only fullscreen styling (not browser fullscreen)
        logsCard.style.position = 'fixed';
        logsCard.style.top = `${topOffset}px`;
        logsCard.style.left = `${leftOffset}px`;
        logsCard.style.width = `${width}px`;
        logsCard.style.height = `${height}px`;
        logsCard.style.zIndex = '50'; // Lower z-index to stay below navigation
        logsCard.style.backgroundColor = '#1f2937';
        logsCard.style.border = 'none';
        logsCard.style.borderRadius = '0';
        logsCard.style.margin = '0';
        logsCard.style.pointerEvents = 'auto'; // Ensure clicks work within the logs area
        logsCard.classList.add('logs-expanded');
        
        console.log('Applied page-only fullscreen styles');
        
        // Create compact header for expanded view
        const header = document.createElement('div');
        header.className = 'logs-fullscreen-header';
        header.innerHTML = `
            <div class="logs-fullscreen-title">
                <i class="fas fa-terminal mr-2"></i>
                System Logs - Expanded View
            </div>
            <div class="logs-fullscreen-controls">
                <div id="fullscreen-stream-status" class="connection-status">
                    <div class="connection-dot disconnected"></div>
                    <span>Disconnected</span>
                </div>
                <button id="fullscreen-toggle-stream" class="btn btn-xs btn-secondary">
                    <i class="fas fa-play mr-1"></i>Stream
                </button>
                <button onclick="window.enhancedLogViewer?.toggleFullscreen()" class="btn btn-xs btn-secondary">
                    <i class="fas fa-compress mr-1"></i>Exit
                </button>
            </div>
        `;
        
        // Insert header at the beginning of the card
        logsCard.insertBefore(header, logsCard.firstChild);
        
        // Update the main fullscreen button to show exit state
        if (this.fullscreenBtn) {
            this.fullscreenBtn.innerHTML = '<i class="fas fa-compress mr-1"></i><span class="hidden sm:inline">Exit</span>';
            this.fullscreenBtn.className = 'btn-log-action btn-warning';
        }
        
        // Copy streaming controls to fullscreen
        this.updateFullscreenControls();
        
        // Focus on logs container for keyboard navigation
        if (this.logContainer) {
            this.logContainer.focus();
        }
        
        // Escape key to exit fullscreen
        this.escapeHandler = (e) => {
            if (e.key === 'Escape') {
                this.exitFullscreen(logsCard);
            }
        };
        document.addEventListener('keydown', this.escapeHandler);
        
        this.showNotification('info', '📺 Expanded logs view - staying within page');
    }
    
    exitFullscreen(logsCard) {
        if (!logsCard) return;
        
        // Remove expanded styling and restore normal positioning
        logsCard.style.position = '';
        logsCard.style.top = '';
        logsCard.style.left = '';
        logsCard.style.width = '';
        logsCard.style.height = '';
        logsCard.style.zIndex = '';
        logsCard.style.backgroundColor = '';
        logsCard.style.border = '';
        logsCard.style.borderRadius = '';
        logsCard.style.margin = '';
        logsCard.classList.remove('logs-expanded');
        
        console.log('Restored normal card styling');
        
        // Remove fullscreen header
        const header = logsCard.querySelector('.logs-fullscreen-header');
        if (header) {
            header.remove();
        }
        
        // Restore the main fullscreen button
        if (this.fullscreenBtn) {
            this.fullscreenBtn.innerHTML = '<i class="fas fa-expand mr-1"></i><span class="hidden sm:inline">Full</span>screen';
            this.fullscreenBtn.className = 'btn-log-action';
        }
        
        // Remove escape key handler
        if (this.escapeHandler) {
            document.removeEventListener('keydown', this.escapeHandler);
            this.escapeHandler = null;
        }
        
        this.showNotification('info', '📱 Exited expanded logs view');
    }
    
    updateFullscreenControls() {
        const fullscreenStreamBtn = document.getElementById('fullscreen-toggle-stream');
        const fullscreenStatus = document.getElementById('fullscreen-stream-status');
        
        if (fullscreenStreamBtn && fullscreenStatus) {
            // Copy current stream status
            if (this.isStreaming) {
                fullscreenStreamBtn.innerHTML = '<i class="fas fa-stop mr-1"></i>Stop';
                fullscreenStreamBtn.className = 'btn btn-sm btn-danger';
                fullscreenStatus.querySelector('.connection-dot').className = 'connection-dot connected';
                fullscreenStatus.querySelector('span').textContent = 'Connected';
            } else {
                fullscreenStreamBtn.innerHTML = '<i class="fas fa-play mr-1"></i>Stream';
                fullscreenStreamBtn.className = 'btn btn-sm btn-secondary';
                fullscreenStatus.querySelector('.connection-dot').className = 'connection-dot disconnected';
                fullscreenStatus.querySelector('span').textContent = 'Disconnected';
            }
            
            // Add click handler
            fullscreenStreamBtn.onclick = () => this.toggleStream();
        }
    }
}

// Initialize enhanced log viewer when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    if (window.location.pathname.includes('/admin/logs')) {
        window.enhancedLogViewer = new EnhancedLogViewer();
        
        // Add export JSON button if not exists
        const exportJsonBtn = document.createElement('button');
        exportJsonBtn.className = 'btn-log-action';
        exportJsonBtn.innerHTML = '<i class="fas fa-file-code mr-2"></i>Export JSON';
        exportJsonBtn.onclick = () => window.enhancedLogViewer.exportLogsAsJSON();
        
        const downloadBtn = document.getElementById('download-logs');
        if (downloadBtn && downloadBtn.parentNode) {
            downloadBtn.parentNode.insertBefore(exportJsonBtn, downloadBtn.nextSibling);
        }
    }
});