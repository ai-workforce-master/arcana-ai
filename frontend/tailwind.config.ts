import type { Config } from 'tailwindcss';
const config: Config = {
  content: ['./app/**/*.{js,ts,jsx,tsx,mdx}', './components/**/*.{js,ts,jsx,tsx,mdx}'],
  theme: { extend: { colors: { mystic: { 900: '#0f0c29', 800: '#1a1540', gold: '#d4af37', silver: '#c0c0c0' } } } },
  plugins: [],
};
export default config;
