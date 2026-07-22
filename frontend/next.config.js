/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  images: {
    remotePatterns: [{ protocol: 'https', hostname: 'ai-arcana.ru', pathname: '/cards/**' }],
  },
};
module.exports = nextConfig;
