package server

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// AdminAuth handles admin authentication
type AdminAuth struct {
	store     *sessions.CookieStore
	templates map[string]*template.Template
}

// NewAdminAuth creates a new admin authentication handler
func NewAdminAuth() *AdminAuth {
	// Generate a random session key if not set
	sessionKey := os.Getenv("ADMIN_SESSION_KEY")
	if sessionKey == "" {
		key := make([]byte, 32)
		rand.Read(key)
		sessionKey = hex.EncodeToString(key)
	}

	store := sessions.NewCookieStore([]byte(sessionKey))
	store.Options = &sessions.Options{
		Path:     "/admin",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   os.Getenv("GIN_MODE") == "release",
	}

	auth := &AdminAuth{
		store:     store,
		templates: make(map[string]*template.Template),
	}

	// Load templates
	auth.loadTemplates()
	return auth
}

// loadTemplates loads all admin templates
func (a *AdminAuth) loadTemplates() {
	templateDir := "templates/admin"

	// Load login template
	loginTmpl, err := template.ParseFiles(filepath.Join(templateDir, "login.html"))
	if err != nil {
		panic("Failed to load login template: " + err.Error())
	}
	a.templates["login"] = loginTmpl

	// Load dashboard templates with layout
	layoutTmpl, err := template.ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "dashboard.html"),
	)
	if err != nil {
		panic("Failed to load dashboard templates: " + err.Error())
	}
	a.templates["dashboard"] = layoutTmpl

	// Load other admin templates
	adminTemplates := []string{"pool", "gap-monitor", "rate-limiter", "logs", "analytics", "alerts", "config", "sessions", "payments"}
	for _, name := range adminTemplates {
		tmplFile := filepath.Join(templateDir, name+".html")
		if _, err := os.Stat(tmplFile); err == nil {
			tmpl, err := template.ParseFiles(
				filepath.Join(templateDir, "layout.html"),
				tmplFile,
			)
			if err == nil {
				a.templates[name] = tmpl
				log.Printf("Successfully loaded template: %s", name)
			} else {
				log.Printf("Error parsing template %s: %v", name, err)
			}
		} else {
			log.Printf("Template file not found: %s", tmplFile)
		}
	}
}

// AdminMiddleware checks if user is authenticated
func (a *AdminAuth) AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for login endpoints
		if c.Request.URL.Path == "/admin/login" {
			c.Next()
			return
		}

		session, _ := a.store.Get(c.Request, "admin-session")

		// Check if user is authenticated
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

// LoginHandler handles the login page and authentication
func (a *AdminAuth) LoginHandler(c *gin.Context) {
	if c.Request.Method == "GET" {
		// Show login form
		data := gin.H{}

		// Check for error messages
		session, _ := a.store.Get(c.Request, "admin-session")
		if flashes := session.Flashes("error"); len(flashes) > 0 {
			data["Error"] = flashes[0]
			session.Save(c.Request, c.Writer)
		}

		c.Header("Content-Type", "text/html")
		a.templates["login"].Execute(c.Writer, data)
		return
	}

	// Handle login POST
	username := c.PostForm("username")
	password := c.PostForm("password")

	// Simple authentication - in production, use proper password hashing
	adminUser := os.Getenv("ADMIN_USERNAME")
	adminPass := os.Getenv("ADMIN_PASSWORD")

	// Default credentials if not set in environment
	if adminUser == "" {
		adminUser = "admin"
	}
	if adminPass == "" {
		adminPass = "paybutton123"
	}

	if username == adminUser && password == adminPass {
		// Successful login
		session, _ := a.store.Get(c.Request, "admin-session")
		session.Values["authenticated"] = true
		session.Values["username"] = username
		session.Save(c.Request, c.Writer)

		c.Redirect(http.StatusFound, "/admin/dashboard")
		return
	}

	// Failed login
	session, _ := a.store.Get(c.Request, "admin-session")
	session.AddFlash("Invalid username or password", "error")
	session.Save(c.Request, c.Writer)

	c.Redirect(http.StatusFound, "/admin/login")
}

// LogoutHandler handles user logout
func (a *AdminAuth) LogoutHandler(c *gin.Context) {
	session, _ := a.store.Get(c.Request, "admin-session")
	session.Values["authenticated"] = false
	delete(session.Values, "username")
	session.Save(c.Request, c.Writer)

	c.Redirect(http.StatusFound, "/admin/login")
}

// DashboardHandler shows the main admin dashboard
func (a *AdminAuth) DashboardHandler(c *gin.Context) {
	session, _ := a.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)

	data := gin.H{
		"Title":      "Dashboard",
		"ActivePage": "dashboard",
		"Username":   username,
	}

	c.Header("Content-Type", "text/html")
	a.templates["dashboard"].Execute(c.Writer, data)
}

// GetFlashMessages retrieves and clears flash messages from session
func (a *AdminAuth) GetFlashMessages(c *gin.Context) ([]string, []string) {
	session, _ := a.store.Get(c.Request, "admin-session")

	var flash, errors []string

	if flashes := session.Flashes("success"); len(flashes) > 0 {
		for _, f := range flashes {
			if str, ok := f.(string); ok {
				flash = append(flash, str)
			}
		}
	}

	if errs := session.Flashes("error"); len(errs) > 0 {
		for _, e := range errs {
			if str, ok := e.(string); ok {
				errors = append(errors, str)
			}
		}
	}

	session.Save(c.Request, c.Writer)
	return flash, errors
}

// SetFlashMessage sets a flash message in the session
func (a *AdminAuth) SetFlashMessage(c *gin.Context, msgType, message string) {
	session, _ := a.store.Get(c.Request, "admin-session")
	session.AddFlash(message, msgType)
	session.Save(c.Request, c.Writer)
}
