package api

import (
	"net/http"
	"os"
	"strings"
)

// corsOrigins returns allowed origins from env or default for dev
func corsOrigins() []string {
	if s := os.Getenv("CORS_ORIGINS"); s != "" {
		return strings.Split(s, ",")
	}
	return []string{"http://localhost:5173", "http://localhost:3000", "http://127.0.0.1:5173", "http://127.0.0.1:3000"}
}

// CORS wraps a handler to add CORS headers
func CORS(next http.Handler) http.Handler {
	origins := corsOrigins()
	originSet := make(map[string]bool)
	for _, o := range origins {
		originSet[strings.TrimSpace(o)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originSet[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(origins) > 0 {
			w.Header().Set("Access-Control-Allow-Origin", strings.TrimSpace(origins[0]))
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
