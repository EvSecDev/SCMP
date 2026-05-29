package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"scmp/internal/global"
	"scmp/web/api"
	"scmp/web/internal"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// Chain middleware handlers together
func chainMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) (chainedHandler http.Handler) {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	chainedHandler = handler
	return
}

type httpLogWriter struct{}

func (logWriter httpLogWriter) Write(message []byte) (msgLen int, err error) {
	fmt.Printf("%s", message) // no newline, http has its own
	msgLen = len(message)
	return
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer      io.Writer
	status      int
	wroteHeader bool
	header      http.Header
	body        *bytes.Buffer
}

func (serverResponder *gzipResponseWriter) WriteHeader(code int) {
	if serverResponder.wroteHeader {
		return
	}
	serverResponder.status = code
	serverResponder.wroteHeader = true
}

func (serverResponder *gzipResponseWriter) Write(b []byte) (int, error) {
	return serverResponder.body.Write(b)
}

func (serverResponder *gzipResponseWriter) Header() http.Header {
	return serverResponder.header
}

func compression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		if !strings.Contains(clientRequest.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(serverResponder, clientRequest)
			return
		}

		gzrw := &gzipResponseWriter{
			ResponseWriter: serverResponder,
			status:         http.StatusOK,
			header:         make(http.Header),
			body:           new(bytes.Buffer),
		}

		next.ServeHTTP(gzrw, clientRequest)

		// If we will compress the response (1xx, 2xx), set header before writing
		if gzrw.status < 300 {
			serverResponder.Header().Set("Content-Encoding", "gzip")
			serverResponder.Header().Del("Content-Length")
		}

		// Copy buffered headers to real response
		for k, vv := range gzrw.header {
			for _, v := range vv {
				serverResponder.Header().Add(k, v)
			}
		}

		// Now write headers
		serverResponder.WriteHeader(gzrw.status)

		if gzrw.status < 300 {
			gz := gzip.NewWriter(serverResponder)
			defer func() {
				_ = gz.Close()
			}()
			_, _ = gz.Write(gzrw.body.Bytes())
		} else {
			_, _ = serverResponder.Write(gzrw.body.Bytes())
		}
	})
}

// Rate limits incoming requests
func rateLimiter(next http.Handler) http.Handler {
	webLimiter := rate.NewLimiter(50, 50) // Limit to 50 requests per sec

	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		if !webLimiter.Allow() {
			http.Error(serverResponder, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(serverResponder, clientRequest)
	})
}

// Add headers that need to be applied to all responses
func addRespHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		// Strict Transport Security (HSTS)
		serverResponder.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Content Security Policy (adjust as needed)
		serverResponder.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:;")

		// Prevent MIME sniffing
		serverResponder.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent click jacking
		serverResponder.Header().Set("X-Frame-Options", "DENY")

		// Referrer Policy
		serverResponder.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions Policy
		serverResponder.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Cache-Control: no caching for API
		if strings.HasPrefix(clientRequest.URL.Path, "/api/") {
			serverResponder.Header().Set("Cache-Control", "no-store")
		}

		next.ServeHTTP(serverResponder, clientRequest)
	})
}

// Validate Request headers
func validateReqHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		// Content-Type check
		if clientRequest.Method == http.MethodPost || clientRequest.Method == http.MethodPut {
			ct := clientRequest.Header.Get("Content-Type")
			if ct != "application/json" && ct != "application/octet-stream" {
				http.Error(serverResponder, "Unsupported Content-Type", http.StatusUnsupportedMediaType)
				return
			}
		}

		userAgent := clientRequest.Header.Get("User-Agent")
		if userAgent == "" {
			http.Error(serverResponder, "Missing User-Agent", http.StatusBadRequest)
			return
		}

		next.ServeHTTP(serverResponder, clientRequest)
	})
}

// Validates JWT from client in all requests
func authentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		// Allow static paths without auth
		allowedPaths := map[string]struct{}{
			"/login.html":  {},
			"/favicon.ico": {},
			"/js/login.js": {},
		}

		// Special handling for JSON-RPC on /api
		if clientRequest.URL.Path == "/api/" && clientRequest.Method == http.MethodPost {
			// Try to decode just enough of the body to get the method name
			var preview struct {
				Method string `json:"method"`
			}

			// We need to preserve the body for the next handler
			var buf bytes.Buffer
			tee := io.TeeReader(clientRequest.Body, &buf)

			// Try decoding only "method" field
			err := json.NewDecoder(tee).Decode(&preview)
			if err == nil && preview.Method == "user.login" {
				// Restore the body for next handler
				clientRequest.Body = io.NopCloser(&buf)
				next.ServeHTTP(serverResponder, clientRequest)
				return
			}

			// Restore the body in all cases
			clientRequest.Body = io.NopCloser(&buf)
		}

		if _, ok := allowedPaths[clientRequest.URL.Path]; ok {
			next.ServeHTTP(serverResponder, clientRequest)
			return
		}

		// Validate JWT cookie
		cookie, err := clientRequest.Cookie("id_token")
		if err != nil {
			http.Redirect(serverResponder, clientRequest, "/login.html", http.StatusFound)
			return
		}

		var parsedToken *jwt.Token
		parsedToken, err = api.VerifyJWT(cookie.Value)
		if err != nil {
			http.Error(serverResponder, "Unauthorized - invalid token", http.StatusUnauthorized)
			return
		}

		claims := parsedToken.Claims.(jwt.MapClaims)
		userName := claims["name"].(string)
		if userName == "" {
			// Mask missing user with unauth
			http.Error(serverResponder, "Unauthorized - unknown user", http.StatusUnauthorized)
		}

		// Retrieve globals for this user to initialize configurations
		users := internal.GetAuthConfig().Users

		var userFound bool
		var userGlbConf internal.UserConfig
		for _, user := range users {
			if user.Username == userName {
				userGlbConf = user
				userFound = true
				break
			}
		}
		if !userFound {
			http.Error(serverResponder,
				fmt.Sprintf("Internal Error - User '%s' has no configuration", userName),
				http.StatusInternalServerError,
			)
			return
		}

		userPermissions := userGlbConf.Permissions

		// Add user configurations to http context
		ctx := context.WithValue(clientRequest.Context(), global.UserKey, userGlbConf.Username)
		ctx = context.WithValue(ctx, global.EmailKey, userGlbConf.Email)
		ctx = context.WithValue(ctx, global.PermKey, userPermissions)
		next.ServeHTTP(serverResponder, clientRequest.WithContext(ctx))
	})
}

// Intercepts all responses and replaces non-2xx/3xx with template page
func customErrorPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		brw := &bufferResponseWriter{ResponseWriter: serverResponder, statusCode: http.StatusOK}
		next.ServeHTTP(brw, clientRequest)

		if brw.statusCode >= 400 {
			// Read your embedded template
			data, err := webFiles.ReadFile("static-files/error.html")
			if err != nil {
				http.Error(serverResponder, "Error page template not found", http.StatusInternalServerError)
				return
			}

			statusText := http.StatusText(brw.statusCode)
			if statusText == "" {
				statusText = "Unknown Error"
			}

			// Prepare values for rendering
			errorHTML := renderErrorTemplate(
				string(data),
				brw.statusCode,
				statusText,
				"Something went wrong while processing your request.",
				brw.buf.String(),
			)

			serverResponder.Header().Set("Content-Type", "text/html; charset=utf-8")
			serverResponder.WriteHeader(brw.statusCode)
			_, _ = serverResponder.Write([]byte(errorHTML))
			return
		}

		// Normal response
		serverResponder.WriteHeader(brw.statusCode)
		_, _ = serverResponder.Write(brw.buf.Bytes())
	})
}

type bufferResponseWriter struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

func (w *bufferResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *bufferResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func renderErrorTemplate(template string, statusCode int, shortDesc, errDesc, fullErr string) string {
	replacer := strings.NewReplacer(
		"{STATUSCODE}", strconv.Itoa(statusCode),
		"{SHORTSTATUSDESCRIPTION}", html.EscapeString(shortDesc),
		"{ERRORDESCRIPTION}", html.EscapeString(errDesc),
		"{FULLERROR}", html.EscapeString(fullErr),
	)
	return replacer.Replace(template)
}
