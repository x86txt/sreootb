/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:8000/api/:path*',
      },
    ]
  },
  experimental: {
    // Enable CSS imports from node_modules for TailwindCSS v4
    optimizePackageImports: ['@tailwindcss/vite']
  },
}

module.exports = nextConfig 