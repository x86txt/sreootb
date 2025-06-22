# Embedding Frontend in Go Server

This document explains how to properly embed and serve the React frontend from your Go server.

## Go Server Configuration

### 1. Embed the Frontend Files

In your `main.go` or server setup file:

```go
package main

import (
    "embed"
    "io/fs"
    "net/http"
    "path/filepath"
    "strings"
)

// Embed the frontend build output
//go:embed frontend/dist
var frontendFS embed.FS

// Embed static assets separately for better caching
//go:embed frontend/dist/_next/static
var staticFS embed.FS
```

### 2. Create File Serving Handlers

```go
// Get the frontend filesystem, stripping the "frontend/dist" prefix
func getFrontendFS() http.FileSystem {
    fsys, err := fs.Sub(frontendFS, "frontend/dist")
    if err != nil {
        panic(err)
    }
    return http.FS(fsys)
}

// Get the static assets filesystem
func getStaticFS() http.FileSystem {
    fsys, err := fs.Sub(staticFS, "frontend/dist/_next/static")
    if err != nil {
        panic(err)
    }
    return http.FS(fsys)
}

// SPA handler that serves index.html for all non-API routes
func spaHandler(fs http.FileSystem) http.HandlerFunc {
    fileServer := http.FileServer(fs)
    
    return func(w http.ResponseWriter, r *http.Request) {
        path := strings.TrimPrefix(r.URL.Path, "/")
        
        // Check if file exists
        if _, err := fs.Open(path); err != nil {
            // File doesn't exist, serve index.html for SPA routing
            r.URL.Path = "/"
        }
        
        fileServer.ServeHTTP(w, r)
    }
}
```

### 3. Setup Routes

```go
func setupRoutes() *http.ServeMux {
    mux := http.NewServeMux()
    
    // API routes
    mux.HandleFunc("/api/", handleAPI)
    
    // Static assets with long-term caching
    staticHandler := http.StripPrefix("/_next/static/", 
        http.FileServer(getStaticFS()))
    mux.Handle("/_next/static/", addCacheHeaders(staticHandler))
    
    // SPA handler for all other routes
    mux.Handle("/", spaHandler(getFrontendFS()))
    
    return mux
}

// Add cache headers for static assets
func addCacheHeaders(handler http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Cache static assets for 1 year
        w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
        handler.ServeHTTP(w, r)
    })
}
```

### 4. Complete Server Example

```go
package main

import (
    "embed"
    "fmt"
    "log"
    "net/http"
    "os"
)

//go:embed frontend/dist
var frontendFS embed.FS

//go:embed frontend/dist/_next/static  
var staticFS embed.FS

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    mux := setupRoutes()
    
    fmt.Printf("Server starting on :%s\n", port)
    fmt.Println("Frontend embedded and ready!")
    
    log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

## Key Points

### 1. **Dual Embed Strategy**
- `frontendFS`: Contains the entire build output
- `staticFS`: Contains just the static assets for optimized caching

### 2. **SPA Routing Support**
- All non-existent routes serve `index.html`
- React Router handles client-side routing
- API routes are preserved and handled by Go

### 3. **Caching Strategy**
- Static assets (`/_next/static/*`): Long-term caching (1 year)
- HTML files: No caching (always fresh)
- API responses: Controlled by your API handlers

### 4. **Path Handling**
- Static assets: `/_next/static/*` → served from `staticFS`
- API routes: `/api/*` → handled by Go API handlers
- Everything else: Served from `frontendFS` with SPA fallback

## Development vs Production

### Development
```bash
# Terminal 1: Go server
go run . server

# Terminal 2: Frontend dev server  
cd frontend && pnpm dev
```

### Production
```bash
# Build everything
./scripts/build.sh linux amd64

# Run single binary
./sreootb server
```

## Testing the Integration

1. **Build the frontend**: `./scripts/build.sh linux amd64`
2. **Run the server**: `./sreootb server`
3. **Test routes**:
   - `/` → Should serve the React app
   - `/agents` → Should serve the React app (SPA routing)
   - `/api/sites` → Should serve API response
   - `/_next/static/...` → Should serve static assets with cache headers

## Troubleshooting

### Frontend Not Loading
- Check that `frontend/dist/` exists and contains files
- Verify embed paths match your actual directory structure
- Ensure static assets are being served from correct path

### API Routes Not Working
- Verify API routes are registered before the SPA handler
- Check that API paths start with `/api/`
- Ensure API handlers return proper HTTP responses

### Routing Issues
- Verify SPA handler serves `index.html` for unknown routes
- Check that static assets don't interfere with app routes
- Ensure trailing slashes are handled consistently 