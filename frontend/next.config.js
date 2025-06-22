/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  trailingSlash: true,
  skipTrailingSlashRedirect: true,
  distDir: '../web',
  images: {
    unoptimized: true,
  },
  typescript: {
    // During static export, ignore TypeScript errors in the types directory
    ignoreBuildErrors: true,
  },
  eslint: {
    // During static export, ignore ESLint errors
    ignoreDuringBuilds: true,
  },
  // Remove rewrites for static export - API calls will go to same origin
  // async rewrites() {
  //   return [
  //     {
  //       source: '/api/:path*',
  //       destination: 'http://localhost:8000/api/:path*',
  //     },
  //   ]
  // },
  experimental: {
    // Enable CSS imports from node_modules for TailwindCSS v4
    optimizePackageImports: ['@tailwindcss/vite'],
    // Disable problematic features during static export
    typedRoutes: false,
  },
}

module.exports = nextConfig 