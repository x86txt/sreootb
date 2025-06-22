# SREootb Frontend

This is the React/Next.js frontend for SREootb, configured to build as a Single Page Application (SPA) that gets embedded into the Go server binary.

## Architecture

The frontend is built as a static SPA using Next.js export mode and embedded into the Go server using Go's `embed` filesystem. This allows for a single binary deployment with all web assets included.

## Build Process

### Automatic Build (Recommended)

The frontend is automatically built when using the main build script:

```bash
# From project root
./scripts/build.sh linux amd64
```

This will:
1. Install frontend dependencies (if needed)
2. Build the frontend SPA
3. Build the Go server with embedded frontend
4. Create a single binary with everything included

### Manual Frontend Build

If you need to build just the frontend:

```bash
cd frontend

# Install dependencies
pnpm install  # or npm install / yarn install

# Build for Go embedding
pnpm run build:go  # or npm run build:go / yarn build:go
```

## Output Structure

The frontend builds to `frontend/dist/` with the following structure:

```
frontend/dist/
├── _next/           # Next.js assets (CSS, JS, etc.)
├── index.html       # Main SPA entry point
└── ...              # Other static assets
```

## Go Integration

The Go server embeds these files using:

```go
//go:embed frontend/dist/_next/static
var staticFS embed.FS

//go:embed frontend/dist
var appFS embed.FS
```

The server serves:
- Static assets (`/_next/*`) from `staticFS`  
- The SPA app from `appFS`
- API routes directly from Go handlers

## Configuration

### Next.js Configuration

- **Output**: `export` mode for static generation
- **Trailing Slash**: Enabled for consistent routing
- **Images**: Unoptimized (no server-side optimization)
- **Directory**: Custom `dist` output directory

### API Integration

- In development: API calls proxy to `localhost:8000`
- In production: API calls go to same origin (embedded server)

## Development

```bash
# Start development server (frontend only)
cd frontend
pnpm dev

# Start full stack development
# Terminal 1: Start Go server
go run . server

# Terminal 2: Start frontend with proxy
cd frontend  
pnpm dev
```

## Scripts

- `build:go` - Build for Go server embedding
- `build:spa` - Standard SPA build
- `clean` - Remove build artifacts
- `dev` - Development server
- `lint` - ESLint checking
- `type-check` - TypeScript checking

## Dependencies

### Runtime
- **React 19** - UI framework
- **Next.js 15** - React framework with static export
- **TailwindCSS** - Styling
- **Radix UI** - Component primitives
- **Recharts** - Charts and visualizations
- **Lucide React** - Icons

### Build Tools
- **TypeScript** - Type safety
- **ESLint** - Code linting
- **PostCSS** - CSS processing
- **Autoprefixer** - CSS vendor prefixes

## Notes

- The frontend is designed to work without server-side rendering
- All routing is handled client-side via React Router
- API authentication uses cookies that work with the Go middleware
- The build process is designed to work in CI/CD environments with minimal dependencies 