// Phase 5: Complete Analytics Dashboard JavaScript
let analyticsData = {};
let charts = {};
let currentPeriod = '24h';
let refreshInterval = null;
let sitesData = {};

// Initialize analytics dashboard
document.addEventListener('DOMContentLoaded', function() {
    initializeAnalytics();
    setupEventListeners();
    startRealTimeUpdates();
});

function initializeAnalytics() {
    loadAnalyticsData();
    initializeCharts();
    setupPhase5Features();
}

function setupEventListeners() {
    // Time period selector
    document.getElementById('timePeriodSelect').addEventListener('change', function(e) {
        currentPeriod = e.target.value;
        refreshAnalytics();
    });
    
    // Historical period selector
    document.getElementById('historicalPeriod').addEventListener('change', function(e) {
        updateHistoricalChart(e.target.value);
    });
    
    // Site filters for Phase 5 features
    document.getElementById('siteFilterPages').addEventListener('change', function(e) {
        updatePageStats(e.target.value);
    });
    
    document.getElementById('siteFilterRegions').addEventListener('change', function(e) {
        updateRegionStats(e.target.value);
    });
}

function loadAnalyticsData() {
    // Load main analytics data
    fetch('/admin/api/site-analytics')
        .then(response => response.json())
        .then(data => {
            analyticsData = data;
            sitesData = data.sites || {};
            updateKPICards(data);
            updateSiteTable(data.sites);
            updateSiteDropdowns();
        })
        .catch(error => console.error('Error loading analytics:', error));
}

function updateKPICards(data) {
    document.getElementById('totalActiveValue').textContent = data.totals?.active || 0;
    document.getElementById('totalWeeklyValue').textContent = data.totals?.weekly || 0;
    document.getElementById('activeSitesValue').textContent = Object.keys(data.sites || {}).length;
}

function updateSiteTable(sites) {
    const tbody = document.getElementById('siteAnalyticsTableBody');
    if (!sites || Object.keys(sites).length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" class="text-center text-gray-500 py-4">No site data available</td></tr>';
        return;
    }

    const rows = Object.values(sites).map(site => `
        <tr class="hover:bg-gray-50 dark:hover:bg-gray-800">
            <td class="py-3 px-4 font-medium">${site.site_name}</td>
            <td class="py-3 px-4 text-center">
                <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                    ${site.active_count}
                </span>
            </td>
            <td class="py-3 px-4 text-center">${site.weekly_total}</td>
            <td class="py-3 px-4 text-center text-xs text-gray-500">
                ${site.last_seen ? new Date(site.last_seen).toLocaleString() : 'Never'}
            </td>
            <td class="py-3 px-4 text-center">
                ${site.active_count > 0 ? 
                    '<span class="inline-flex items-center px-2 py-1 rounded-full text-xs bg-green-100 text-green-800"><div class="w-1.5 h-1.5 bg-green-500 rounded-full mr-1.5"></div>Active</span>' :
                    '<span class="inline-flex items-center px-2 py-1 rounded-full text-xs bg-gray-100 text-gray-800">Inactive</span>'
                }
            </td>
        </tr>
    `).join('');
    
    tbody.innerHTML = rows;
}

function updateSiteDropdowns() {
    const siteNames = Object.keys(sitesData);
    
    // Update all dropdowns
    ['exportSiteSelect', 'siteFilterPages', 'siteFilterRegions'].forEach(selectId => {
        const select = document.getElementById(selectId);
        const currentValue = select.value;
        
        select.innerHTML = select.id === 'exportSiteSelect' ? 
            '<option value="">Select a site...</option>' :
            '<option value="">All Sites</option>';
            
        siteNames.forEach(siteName => {
            const option = document.createElement('option');
            option.value = siteName;
            option.textContent = siteName;
            select.appendChild(option);
        });
        
        select.value = currentValue;
    });
}

// Phase 5: Historical Chart
function updateHistoricalChart(hours) {
    if (Object.keys(sitesData).length === 0) return;
    
    const siteName = Object.keys(sitesData)[0]; // Use first site as example
    
    fetch(`/admin/api/site-analytics/${siteName}/historical?hours=${hours}`)
        .then(response => response.json())
        .then(data => {
            const ctx = document.getElementById('historicalChart').getContext('2d');
            
            if (charts.historical) {
                charts.historical.destroy();
            }
            
            const labels = data.historical_data.map(point => 
                new Date(point.timestamp).toLocaleString()
            );
            const values = data.historical_data.map(point => point.viewers);
            
            charts.historical = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [{
                        label: 'Active Viewers',
                        data: values,
                        borderColor: '#4f46e5',
                        backgroundColor: 'rgba(79, 70, 229, 0.1)',
                        fill: true,
                        tension: 0.4
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: {
                        y: {
                            beginAtZero: true
                        }
                    }
                }
            });
            
            // Update stats
            if (values.length > 0) {
                const avg = Math.round(values.reduce((a, b) => a + b, 0) / values.length);
                const max = Math.max(...values);
                document.getElementById('avgViewersText').textContent = avg;
                document.getElementById('peakViewersText').textContent = max;
                document.getElementById('trendText').textContent = 
                    values[values.length - 1] > values[0] ? '↗ Rising' : '↘ Declining';
            }
        })
        .catch(error => console.error('Error loading historical data:', error));
}

// Phase 5: Page Statistics
function updatePageStats(siteName) {
    if (!siteName) {
        document.getElementById('pagesTableBody').innerHTML = 
            '<tr><td colspan="3" class="text-center text-gray-500 py-4">Select a site to view page statistics</td></tr>';
        return;
    }
    
    fetch(`/admin/api/site-analytics/${siteName}/pages?limit=10`)
        .then(response => response.json())
        .then(data => {
            const tbody = document.getElementById('pagesTableBody');
            
            if (!data.page_stats || data.page_stats.length === 0) {
                tbody.innerHTML = '<tr><td colspan="3" class="text-center text-gray-500 py-4">No page data available</td></tr>';
                return;
            }
            
            const rows = data.page_stats.map(page => `
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-800">
                    <td class="py-2 px-3 font-mono text-sm">${page.path}</td>
                    <td class="py-2 px-3 text-center">${page.views}</td>
                    <td class="py-2 px-3 text-center">${page.unique}</td>
                </tr>
            `).join('');
            
            tbody.innerHTML = rows;
        })
        .catch(error => console.error('Error loading page stats:', error));
}

// Phase 5: Region Statistics
function updateRegionStats(siteName) {
    if (!siteName) {
        document.getElementById('regionsTableBody').innerHTML = 
            '<tr><td colspan="3" class="text-center text-gray-500 py-4">Select a site to view region statistics</td></tr>';
        return;
    }
    
    fetch(`/admin/api/site-analytics/${siteName}/regions`)
        .then(response => response.json())
        .then(data => {
            // Update table
            const tbody = document.getElementById('regionsTableBody');
            
            if (!data.region_stats || data.region_stats.length === 0) {
                tbody.innerHTML = '<tr><td colspan="3" class="text-center text-gray-500 py-4">No region data available</td></tr>';
                return;
            }
            
            const rows = data.region_stats.map(region => `
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-800">
                    <td class="py-2 px-3">${region.region}</td>
                    <td class="py-2 px-3 text-center">${region.count}</td>
                    <td class="py-2 px-3 text-center">${region.percentage.toFixed(1)}%</td>
                </tr>
            `).join('');
            
            tbody.innerHTML = rows;
            
            // Update chart
            updateRegionChart(data.region_stats);
        })
        .catch(error => console.error('Error loading region stats:', error));
}

function updateRegionChart(regionStats) {
    const ctx = document.getElementById('regionsChart').getContext('2d');
    
    if (charts.regions) {
        charts.regions.destroy();
    }
    
    charts.regions = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: regionStats.map(r => r.region),
            datasets: [{
                data: regionStats.map(r => r.count),
                backgroundColor: [
                    '#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6',
                    '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16'
                ]
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false }
            }
        }
    });
}

// Phase 5: Export functionality
function exportSiteData(format) {
    const siteName = document.getElementById('exportSiteSelect').value;
    const period = document.getElementById('exportPeriodSelect').value;
    
    if (!siteName) {
        showExportStatus('Please select a site first', 'error');
        return;
    }
    
    showExportStatus('Preparing export...', 'info');
    
    const url = `/admin/api/site-analytics/${siteName}/export?period=${period}&format=${format}&download=true`;
    
    // Create download link
    const link = document.createElement('a');
    link.href = url;
    link.download = `${siteName}-analytics-${period}.${format}`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    
    showExportStatus(`Export completed: ${siteName}-analytics-${period}.${format}`, 'success');
}

function showExportStatus(message, type) {
    const statusDiv = document.getElementById('exportStatus');
    const icon = type === 'success' ? 'check-circle' : 
                type === 'error' ? 'exclamation-circle' : 'info-circle';
    const color = type === 'success' ? 'text-green-500' : 
                 type === 'error' ? 'text-red-500' : 'text-blue-500';
    
    statusDiv.innerHTML = `
        <div class="flex items-center">
            <i class="fas fa-${icon} ${color} mr-2"></i>
            <span class="text-sm">${message}</span>
        </div>
    `;
    statusDiv.style.display = 'block';
    
    if (type === 'success' || type === 'error') {
        setTimeout(() => {
            statusDiv.style.display = 'none';
        }, 3000);
    }
}

// Setup Phase 5 features
function setupPhase5Features() {
    // Load historical chart
    updateHistoricalChart(720); // Default 30 days
    
    // Initialize empty states
    updatePageStats('');
    updateRegionStats('');
}

// Initialize charts
function initializeCharts() {
    // Main charts will be updated when data loads
    loadAnalyticsData();
}

// Real-time updates
function startRealTimeUpdates() {
    refreshInterval = setInterval(() => {
        loadAnalyticsData();
        // Update last updated timestamp
        document.getElementById('lastUpdated').textContent = new Date().toLocaleString();
    }, 5000); // Update every 5 seconds
}

function refreshAnalytics() {
    loadAnalyticsData();
    if (Object.keys(sitesData).length > 0) {
        updateHistoricalChart(document.getElementById('historicalPeriod').value);
    }
}

// Export function for button
function exportAnalytics() {
    exportSiteData('json');
}