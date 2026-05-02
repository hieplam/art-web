// web/next.config.js
/** @type {import('next').NextConfig} */
module.exports = {
  reactStrictMode: true,
  images: {
    deviceSizes: [240, 480, 800, 1024, 1600, 2400],
    imageSizes: [],
    loaderFile: "./lib/cf-loader.ts",
    remotePatterns: [
      { protocol: "https", hostname: "cdn.example.com" },
      { protocol: "http",  hostname: "localhost" },
    ],
  },
};
