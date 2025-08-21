// Configuration Management JavaScript
class ConfigurationManager {
    constructor() {
        this.currentConfig = null;
        this.originalConfig = null;
        this.pendingChanges = new Map();
        this.validationRules = null;
        this.activeTab = 'pool';
        
        this.init();
    }

    async init() {
        this.setupEventListeners();
        await this.loadConfiguration();
        await this.loadValidationRules();
        this.setupTabSwitching();
        this.setupFormValidation();
    }

    setupEventListeners() {
        // Tab switching
        document.querySelectorAll('.config-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.switchTab(e.target.dataset.tab);
            });
        });

        // Form change detection
        document.querySelectorAll('.config-section input, .config-section select, .config-section textarea').forEach(input => {
            input.addEventListener('input', () => {
                this.detectChanges();
            });
        });

        // Prevent form submission on enter
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && e.target.tagName === 'INPUT') {
                e.preventDefault();
            }
        });
    }

    async loadConfiguration() {
        try {
            showLoading('Loading configuration...');
            const response = await fetch('/admin/api/config/current');
            
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            
            this.currentConfig = await response.json();
            this.originalConfig = JSON.parse(JSON.stringify(this.currentConfig));
            
            this.populateForm();
            this.updateStatus();
            hideLoading();
        } catch (error) {
            console.error('Error loading configuration:', error);
            showNotification('Failed to load configuration', 'error');
            hideLoading();
        }
    }

    async loadValidationRules() {
        try {
            const response = await fetch('/admin/api/config/validation-rules');
            this.validationRules = await response.json();
        } catch (error) {
            console.error('Error loading validation rules:', error);
        }
    }

    populateForm() {
        if (!this.currentConfig) return;

        // Address Pool Configuration
        document.getElementById('poolMinSize').value = this.currentConfig.address_pool.min_pool_size;
        document.getElementById('poolMaxSize').value = this.currentConfig.address_pool.max_pool_size;
        document.getElementById('poolRefillThreshold').value = this.currentConfig.address_pool.refill_threshold;
        document.getElementById('poolRefillBatch').value = this.currentConfig.address_pool.refill_batch_size;
        document.getElementById('poolCleanupInterval').value = this.currentConfig.address_pool.cleanup_interval_minutes;

        // Gap Monitor Configuration
        document.getElementById('gapMaxLimit').value = this.currentConfig.gap_monitor.max_gap_limit;
        document.getElementById('gapWarningThreshold').value = Math.round(this.currentConfig.gap_monitor.warning_threshold * 100);
        document.getElementById('gapCriticalThreshold').value = Math.round(this.currentConfig.gap_monitor.critical_threshold * 100);
        document.getElementById('gapFailThreshold').value = this.currentConfig.gap_monitor.consecutive_fail_threshold;
        document.getElementById('gapErrorHistory').value = this.currentConfig.gap_monitor.error_history_size;

        // Rate Limiter Configuration
        document.getElementById('rateLimitGlobalMax').value = this.currentConfig.rate_limiter.global_max_tokens;
        document.getElementById('rateLimitGlobalRefill').value = this.currentConfig.rate_limiter.global_refill_rate;
        document.getElementById('rateLimitIPMax').value = this.currentConfig.rate_limiter.ip_max_tokens;
        document.getElementById('rateLimitIPRefill').value = this.currentConfig.rate_limiter.ip_refill_rate;
        document.getElementById('rateLimitEmailMax').value = this.currentConfig.rate_limiter.email_max_tokens;
        document.getElementById('rateLimitEmailRefill').value = this.currentConfig.rate_limiter.email_refill_rate;
        document.getElementById('rateLimitCleanup').value = this.currentConfig.rate_limiter.cleanup_interval_minutes;

        // WebSocket Configuration
        document.getElementById('wsReadBuffer').value = this.currentConfig.websocket.read_buffer_size;
        document.getElementById('wsWriteBuffer').value = this.currentConfig.websocket.write_buffer_size;
        document.getElementById('wsPingInterval').value = this.currentConfig.websocket.ping_interval_seconds;
        document.getElementById('wsPongTimeout').value = this.currentConfig.websocket.pong_timeout_seconds;
        document.getElementById('wsMaxConnections').value = this.currentConfig.websocket.max_connections;

        // Admin Dashboard Configuration
        document.getElementById('adminSessionTimeout').value = this.currentConfig.admin_dashboard.session_timeout_hours;
        document.getElementById('adminRefreshInterval').value = this.currentConfig.admin_dashboard.refresh_interval_seconds;
        document.getElementById('adminMaxLogs').value = this.currentConfig.admin_dashboard.max_log_entries;
        document.getElementById('adminMetricsRetention').value = this.currentConfig.admin_dashboard.metrics_retention_hours;
        document.getElementById('adminEnableMetrics').checked = this.currentConfig.admin_dashboard.enable_metrics;
        document.getElementById('adminEnableAlerts').checked = this.currentConfig.admin_dashboard.enable_alerts;
    }

    updateStatus() {
        if (!this.currentConfig) return;

        document.getElementById('configVersion').textContent = this.currentConfig.version;
        
        const lastUpdated = new Date(this.currentConfig.timestamp);
        document.getElementById('lastUpdated').textContent = this.formatTimeAgo(lastUpdated);
        
        document.getElementById('pendingChanges').textContent = this.pendingChanges.size;
    }

    formatTimeAgo(date) {
        const now = new Date();
        const diffInMinutes = Math.floor((now - date) / (1000 * 60));
        
        if (diffInMinutes < 1) return 'Just now';
        if (diffInMinutes < 60) return `${diffInMinutes}m ago`;
        
        const diffInHours = Math.floor(diffInMinutes / 60);
        if (diffInHours < 24) return `${diffInHours}h ago`;
        
        const diffInDays = Math.floor(diffInHours / 24);
        return `${diffInDays}d ago`;
    }

    detectChanges() {
        const currentFormData = this.getFormData();
        const hasChanges = !this.deepEqual(currentFormData, this.originalConfig);
        
        const saveButton = document.getElementById('saveAllBtn');
        saveButton.disabled = !hasChanges;
        
        document.getElementById('pendingChanges').textContent = hasChanges ? '1' : '0';
    }

    getFormData() {
        return {
            address_pool: {
                min_pool_size: parseInt(document.getElementById('poolMinSize').value),
                max_pool_size: parseInt(document.getElementById('poolMaxSize').value),
                refill_threshold: parseInt(document.getElementById('poolRefillThreshold').value),
                refill_batch_size: parseInt(document.getElementById('poolRefillBatch').value),
                cleanup_interval_minutes: parseInt(document.getElementById('poolCleanupInterval').value)
            },
            gap_monitor: {
                max_gap_limit: parseInt(document.getElementById('gapMaxLimit').value),
                warning_threshold: parseFloat(document.getElementById('gapWarningThreshold').value) / 100,
                critical_threshold: parseFloat(document.getElementById('gapCriticalThreshold').value) / 100,
                consecutive_fail_threshold: parseInt(document.getElementById('gapFailThreshold').value),
                error_history_size: parseInt(document.getElementById('gapErrorHistory').value)
            },
            rate_limiter: {
                global_max_tokens: parseInt(document.getElementById('rateLimitGlobalMax').value),
                global_refill_rate: parseInt(document.getElementById('rateLimitGlobalRefill').value),
                ip_max_tokens: parseInt(document.getElementById('rateLimitIPMax').value),
                ip_refill_rate: parseInt(document.getElementById('rateLimitIPRefill').value),
                email_max_tokens: parseInt(document.getElementById('rateLimitEmailMax').value),
                email_refill_rate: parseInt(document.getElementById('rateLimitEmailRefill').value),
                cleanup_interval_minutes: parseInt(document.getElementById('rateLimitCleanup').value)
            },
            websocket: {
                read_buffer_size: parseInt(document.getElementById('wsReadBuffer').value),
                write_buffer_size: parseInt(document.getElementById('wsWriteBuffer').value),
                ping_interval_seconds: parseInt(document.getElementById('wsPingInterval').value),
                pong_timeout_seconds: parseInt(document.getElementById('wsPongTimeout').value),
                max_connections: parseInt(document.getElementById('wsMaxConnections').value)
            },
            admin_dashboard: {
                session_timeout_hours: parseInt(document.getElementById('adminSessionTimeout').value),
                refresh_interval_seconds: parseInt(document.getElementById('adminRefreshInterval').value),
                max_log_entries: parseInt(document.getElementById('adminMaxLogs').value),
                metrics_retention_hours: parseInt(document.getElementById('adminMetricsRetention').value),
                enable_metrics: document.getElementById('adminEnableMetrics').checked,
                enable_alerts: document.getElementById('adminEnableAlerts').checked
            }
        };
    }

    setupTabSwitching() {
        const tabs = document.querySelectorAll('.config-tab');
        const sections = document.querySelectorAll('.config-section');

        tabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const tabName = tab.dataset.tab;
                this.switchTab(tabName);
            });
        });
    }

    switchTab(tabName) {
        // Update active tab
        document.querySelectorAll('.config-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');

        // Update active section
        document.querySelectorAll('.config-section').forEach(section => {
            section.classList.remove('active');
        });
        document.getElementById(`${tabName}-config`).classList.add('active');

        this.activeTab = tabName;

        // Load history if switching to history tab
        if (tabName === 'history') {
            this.loadConfigurationHistory();
        }
    }

    setupFormValidation() {
        // Add real-time validation
        document.querySelectorAll('.config-section input[type="number"]').forEach(input => {
            input.addEventListener('blur', () => {
                this.validateField(input);
            });
            
            input.addEventListener('input', () => {
                this.clearFieldError(input);
            });
        });

        // Cross-field validation
        document.getElementById('gapWarningThreshold').addEventListener('input', () => {
            this.validateThresholds();
        });
        document.getElementById('gapCriticalThreshold').addEventListener('input', () => {
            this.validateThresholds();
        });

        document.getElementById('poolMinSize').addEventListener('input', () => {
            this.validatePoolSizes();
        });
        document.getElementById('poolMaxSize').addEventListener('input', () => {
            this.validatePoolSizes();
        });
    }

    validateField(input) {
        const value = parseFloat(input.value);
        const min = parseFloat(input.min);
        const max = parseFloat(input.max);

        if (isNaN(value)) {
            this.showFieldError(input, 'Please enter a valid number');
            return false;
        }

        if (min !== undefined && value < min) {
            this.showFieldError(input, `Value must be at least ${min}`);
            return false;
        }

        if (max !== undefined && value > max) {
            this.showFieldError(input, `Value must be no more than ${max}`);
            return false;
        }

        this.clearFieldError(input);
        return true;
    }

    validateThresholds() {
        const warning = parseFloat(document.getElementById('gapWarningThreshold').value);
        const critical = parseFloat(document.getElementById('gapCriticalThreshold').value);

        if (warning >= critical) {
            this.showFieldError(document.getElementById('gapCriticalThreshold'), 
                'Critical threshold must be higher than warning threshold');
            return false;
        }

        this.clearFieldError(document.getElementById('gapWarningThreshold'));
        this.clearFieldError(document.getElementById('gapCriticalThreshold'));
        return true;
    }

    validatePoolSizes() {
        const minSize = parseInt(document.getElementById('poolMinSize').value);
        const maxSize = parseInt(document.getElementById('poolMaxSize').value);

        if (maxSize <= minSize) {
            this.showFieldError(document.getElementById('poolMaxSize'), 
                'Max pool size must be greater than min pool size');
            return false;
        }

        this.clearFieldError(document.getElementById('poolMinSize'));
        this.clearFieldError(document.getElementById('poolMaxSize'));
        return true;
    }

    showFieldError(input, message) {
        this.clearFieldError(input);
        
        input.classList.add('error');
        const errorDiv = document.createElement('div');
        errorDiv.className = 'field-error text-red-600 text-sm mt-1';
        errorDiv.textContent = message;
        input.parentNode.appendChild(errorDiv);
    }

    clearFieldError(input) {
        input.classList.remove('error');
        const errorDiv = input.parentNode.querySelector('.field-error');
        if (errorDiv) {
            errorDiv.remove();
        }
    }

    async saveConfiguration() {
        if (!this.validateAllFields()) {
            showNotification('Please fix validation errors before saving', 'error');
            return;
        }

        const newConfig = this.getFormData();
        const changes = this.getConfigurationChanges(this.originalConfig, newConfig);

        if (changes.length === 0) {
            showNotification('No changes to save', 'info');
            return;
        }

        this.showConfirmationModal(changes);
    }

    validateAllFields() {
        let isValid = true;
        
        // Validate all number inputs
        document.querySelectorAll('.config-section input[type="number"]').forEach(input => {
            if (!this.validateField(input)) {
                isValid = false;
            }
        });

        // Validate cross-field constraints
        if (!this.validateThresholds()) isValid = false;
        if (!this.validatePoolSizes()) isValid = false;

        return isValid;
    }

    getConfigurationChanges(oldConfig, newConfig) {
        const changes = [];
        
        this.compareSection(oldConfig.address_pool, newConfig.address_pool, 'Address Pool', changes);
        this.compareSection(oldConfig.gap_monitor, newConfig.gap_monitor, 'Gap Monitor', changes);
        this.compareSection(oldConfig.rate_limiter, newConfig.rate_limiter, 'Rate Limiter', changes);
        this.compareSection(oldConfig.websocket, newConfig.websocket, 'WebSocket', changes);
        this.compareSection(oldConfig.admin_dashboard, newConfig.admin_dashboard, 'Admin Dashboard', changes);
        
        return changes;
    }

    compareSection(oldSection, newSection, sectionName, changes) {
        for (const [key, newValue] of Object.entries(newSection)) {
            const oldValue = oldSection[key];
            if (oldValue !== newValue) {
                changes.push({
                    section: sectionName,
                    field: this.formatFieldName(key),
                    oldValue: oldValue,
                    newValue: newValue
                });
            }
        }
    }

    formatFieldName(fieldName) {
        return fieldName.replace(/_/g, ' ')
            .replace(/\b\w/g, l => l.toUpperCase());
    }

    showConfirmationModal(changes) {
        const modal = document.getElementById('confirmationModal');
        const message = document.getElementById('confirmationMessage');
        const summary = document.getElementById('changesSummary');

        message.textContent = `You are about to save ${changes.length} configuration changes.`;
        
        summary.innerHTML = changes.map(change => 
            `<div class="flex justify-between items-center py-2 border-b border-gray-200 last:border-b-0">
                <div>
                    <span class="font-medium">${change.section}</span> - ${change.field}
                </div>
                <div class="text-sm">
                    <span class="text-red-600">${change.oldValue}</span> → 
                    <span class="text-green-600">${change.newValue}</span>
                </div>
            </div>`
        ).join('');

        modal.classList.remove('hidden');
    }

    async confirmConfigurationChange() {
        const description = document.getElementById('changeDescription').value.trim();
        if (!description) {
            showNotification('Please provide a description for this change', 'error');
            return;
        }

        try {
            showLoading('Saving configuration...');
            
            const newConfig = this.getFormData();
            const response = await fetch('/admin/api/config/update', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    config: newConfig,
                    description: description
                })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to save configuration');
            }

            const result = await response.json();
            
            this.closeModal();
            hideLoading();
            showNotification('Configuration saved successfully', 'success');
            
            // Reload configuration
            await this.loadConfiguration();
            
        } catch (error) {
            console.error('Error saving configuration:', error);
            hideLoading();
            showNotification(error.message, 'error');
        }
    }

    async loadConfigurationHistory() {
        try {
            const response = await fetch('/admin/api/config/history');
            const history = await response.json();
            
            this.displayConfigurationHistory(history);
        } catch (error) {
            console.error('Error loading configuration history:', error);
            document.getElementById('configHistory').innerHTML = 
                '<div class="text-center py-8 text-red-600">Failed to load configuration history</div>';
        }
    }

    displayConfigurationHistory(history) {
        const container = document.getElementById('configHistory');
        
        if (!history || history.length === 0) {
            container.innerHTML = 
                '<div class="text-center py-8 text-gray-500">No configuration changes found</div>';
            return;
        }

        container.innerHTML = history.map(change => `
            <div class="border border-gray-200 rounded-lg p-4 ${change.success ? 'bg-white' : 'bg-red-50 border-red-200'}">
                <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-3">
                    <div class="flex items-center space-x-3">
                        <div class="flex-shrink-0">
                            <i class="fas ${change.success ? 'fa-check-circle text-green-500' : 'fa-times-circle text-red-500'}"></i>
                        </div>
                        <div>
                            <h4 class="text-sm font-medium text-gray-900">${change.description}</h4>
                            <p class="text-sm text-gray-600">
                                by ${change.changed_by} • ${this.formatTimeAgo(new Date(change.timestamp))}
                            </p>
                        </div>
                    </div>
                    <div class="flex space-x-2 mt-3 sm:mt-0">
                        ${change.success && change.prev_config ? 
                            `<button onclick="configManager.showRollbackModal('${change.id}')" 
                                    class="btn-sm btn-secondary">
                                <i class="fas fa-undo mr-1"></i>Rollback
                            </button>` : ''
                        }
                    </div>
                </div>
                
                ${change.error_msg ? 
                    `<div class="bg-red-100 border border-red-200 rounded p-3 mb-3">
                        <p class="text-sm text-red-700">${change.error_msg}</p>
                    </div>` : ''
                }
                
                ${change.changes && change.changes.length > 0 ? 
                    `<div class="bg-gray-50 rounded p-3">
                        <h5 class="text-sm font-medium text-gray-900 mb-2">Changes:</h5>
                        <div class="space-y-1">
                            ${change.changes.map(fieldChange => 
                                `<div class="text-sm text-gray-600">
                                    <strong>${fieldChange.field}:</strong> 
                                    <span class="text-red-600">${fieldChange.old_value}</span> → 
                                    <span class="text-green-600">${fieldChange.new_value}</span>
                                </div>`
                            ).join('')}
                        </div>
                    </div>` : ''
                }
            </div>
        `).join('');
    }

    closeModal() {
        document.getElementById('confirmationModal').classList.add('hidden');
        document.getElementById('changeDescription').value = '';
    }

    deepEqual(obj1, obj2) {
        return JSON.stringify(obj1) === JSON.stringify(obj2);
    }
}

// Global functions for button clicks
async function saveAllConfiguration() {
    await configManager.saveConfiguration();
}

async function resetToDefaults() {
    if (confirm('Are you sure you want to reset all configuration to default values? This will discard any unsaved changes.')) {
        try {
            showLoading('Resetting to defaults...');
            
            const response = await fetch('/admin/api/config/reset-defaults', {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error('Failed to reset configuration');
            }

            await configManager.loadConfiguration();
            showNotification('Configuration reset to defaults', 'success');
        } catch (error) {
            console.error('Error resetting configuration:', error);
            showNotification('Failed to reset configuration', 'error');
        } finally {
            hideLoading();
        }
    }
}

function refreshHistory() {
    configManager.loadConfigurationHistory();
}

function confirmConfigurationChange() {
    configManager.confirmConfigurationChange();
}

function closeModal() {
    configManager.closeModal();
}

// Helper functions
function showLoading(message) {
    // Show loading indicator (implement based on your UI framework)
    console.log('Loading:', message);
}

function hideLoading() {
    // Hide loading indicator
    console.log('Loading complete');
}

function showNotification(message, type = 'info') {
    // Show notification (implement based on your UI framework)
    console.log(`${type.toUpperCase()}: ${message}`);
    
    // Simple browser notification for now
    if (type === 'error') {
        alert('Error: ' + message);
    } else if (type === 'success') {
        // Could use a toast library here
        console.log('Success:', message);
    }
}

// Initialize configuration manager when DOM is loaded
let configManager;
document.addEventListener('DOMContentLoaded', () => {
    configManager = new ConfigurationManager();
});