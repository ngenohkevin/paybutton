# PayButton Admin UI

A comprehensive web-based admin interface for monitoring and managing the PayButton payment processing system with gap limit protection.

## Features

### 🔐 **Secure Authentication**
- Session-based login system
- Configurable admin credentials via environment variables
- Auto-logout on session expiry
- Remember me functionality

### 📊 **Real-time Dashboard**
- Live system status monitoring
- Interactive charts for pool, gap monitor, and rate limiter
- Auto-refresh every 30 seconds
- Quick action buttons
- System health indicators

### 🏊 **Address Pool Management**
- View pool statistics (available, used, recycled addresses)
- Manual pool refill capability
- Real-time pool size monitoring
- Address recycling status

### ⚠️ **Gap Limit Monitoring**
- Real-time gap ratio tracking
- Paid vs unpaid address visualization
- Fallback mode status
- Manual gap counter reset
- Alert thresholds monitoring

### 🛡️ **Rate Limiter Control**
- Global, IP, and email rate limit monitoring
- Active limits tracking
- Token bucket status
- User rate limit reset functionality

### 🎨 **Modern UI/UX**
- Responsive design with Tailwind CSS
- Real-time updates with HTMX
- Interactive charts with Chart.js
- Font Awesome icons
- Clean, professional interface

## Setup & Configuration

### Environment Variables

Add these to your `.env` file:

```bash
# Admin UI Authentication
ADMIN_USERNAME=admin                    # Default: admin
ADMIN_PASSWORD=your_secure_password     # Default: paybutton123
ADMIN_SESSION_KEY=your_random_32byte_key # Auto-generated if not set

# Optional: Set to true for HTTPS-only cookies in production
GIN_MODE=release
```

### Default Credentials (if not configured)
- **Username:** `admin`
- **Password:** `paybutton123`

⚠️ **Security Note:** Change the default credentials before deploying to production!

## Access

Once the server is running, access the admin interface at:

```
http://localhost:8080/admin/login
```

## Navigation

### Main Sections
1. **Dashboard** (`/admin/dashboard`) - System overview and real-time monitoring
2. **Address Pool** (`/admin/pool`) - Pool management and statistics  
3. **Gap Monitor** (`/admin/gap-monitor`) - Gap limit monitoring and control
4. **Rate Limiter** (`/admin/rate-limiter`) - Rate limiting management

### API Endpoints

The admin UI also exposes JSON API endpoints for external monitoring:

```bash
# System status
GET /admin/status

# Address pool stats  
GET /admin/pool/stats
POST /admin/pool/refill

# Gap monitor stats
GET /admin/gap/stats
POST /admin/gap/reset

# Rate limiter stats
GET /admin/ratelimit/stats  
POST /admin/ratelimit/reset/:email
```

## Dashboard Features

### Real-time Monitoring
- **System Status:** Overall health indicator
- **Address Pool:** Available vs used addresses with chart
- **Gap Monitor:** Paid vs unpaid ratio with fallback status
- **Rate Limiter:** Token availability and active limits

### Interactive Charts
- Doughnut charts for visual representation
- Real-time data updates
- Color-coded status indicators

### Quick Actions
- **Refill Pool:** Manually trigger address pool refill
- **Reset Gap:** Reset gap limit counter  
- **View Logs:** Access system logs (placeholder)
- **Export Stats:** Download system statistics

### Keyboard Shortcuts
- `Ctrl/Cmd + R`: Refresh dashboard
- `Ctrl/Cmd + P`: Refill pool  
- `Ctrl/Cmd + G`: Reset gap counter

## Technical Details

### Architecture
- **Backend:** Go with Gin framework
- **Frontend:** HTMX for real-time updates
- **Styling:** Tailwind CSS
- **Charts:** Chart.js
- **Icons:** Font Awesome
- **Authentication:** Gorilla sessions

### File Structure
```
templates/admin/
├── layout.html      # Base template with navigation
├── login.html       # Login page
└── dashboard.html   # Main dashboard

static/
├── js/
│   └── admin.js     # Dashboard JavaScript
└── css/             # Custom styles (if needed)

internals/server/
├── admin_auth.go     # Authentication system
└── admin_endpoints.go # Admin API endpoints
```

### Security Features
- Session-based authentication
- CSRF protection via secure cookies
- HTTP-only session cookies  
- Secure cookie flags in production
- Session timeout (7 days default)
- Login attempt protection

## Monitoring & Alerts

The system provides comprehensive monitoring:

### Status Indicators
- 🟢 **Healthy:** All systems operational
- 🟡 **Warning:** Approaching limits
- 🔴 **Critical:** Intervention needed

### Recommendations Engine
- Dynamic recommendations based on system state
- Actionable insights for optimization
- Proactive problem identification

### Real-time Notifications
- In-browser notifications for actions
- Success/error feedback
- Auto-dismissing alerts

## Troubleshooting

### Common Issues

**Login not working:**
- Check `ADMIN_USERNAME` and `ADMIN_PASSWORD` environment variables
- Verify session key is set properly
- Clear browser cookies and try again

**Dashboard not loading:**
- Ensure all templates are in the correct directory
- Check server logs for template parsing errors
- Verify static files are being served correctly

**Charts not displaying:**
- Check browser console for JavaScript errors
- Ensure Chart.js is loading from CDN
- Verify API endpoints are returning data

**Auto-refresh not working:**
- Check HTMX library is loaded
- Verify `/admin/api/status` endpoint is accessible
- Check browser network tab for failed requests

### Logs & Debugging

System logs will show:
- Admin authentication attempts
- API endpoint access
- Error messages
- Performance metrics

## Production Deployment

### Security Checklist
- [ ] Change default admin credentials
- [ ] Set secure session key
- [ ] Enable HTTPS
- [ ] Set `GIN_MODE=release`  
- [ ] Configure proper firewall rules
- [ ] Enable access logging
- [ ] Set up monitoring alerts

### Performance Optimization
- [ ] Enable gzip compression
- [ ] Set up CDN for static assets
- [ ] Configure caching headers
- [ ] Monitor memory usage
- [ ] Set up health checks

## Development

To extend the admin UI:

1. **Add new pages:** Create templates in `templates/admin/`
2. **Add endpoints:** Extend `admin_endpoints.go`
3. **Add JavaScript:** Modify `static/js/admin.js`
4. **Add styles:** Use Tailwind classes or add custom CSS

The system is designed to be easily extensible while maintaining security and performance.