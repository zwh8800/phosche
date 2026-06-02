import {
  defineConfig,
  minimal2023Preset,
} from '@vite-pwa/assets-generator/config';

const preset = {
  ...minimal2023Preset,
  maskable: {
    sizes: [512],
    padding: 0.1,
    resizeOptions: {
      fit: 'contain',
      background: 'transparent',
    },
  },
  apple: {
    sizes: [180],
    padding: 0.1,
    resizeOptions: {
      fit: 'contain',
      background: 'transparent',
    },
  },
};

export default defineConfig({
  preset,
  images: 'public/favicon.svg',
  headLinkOptions: {
    preset: '2023',
  },
  overrideAssets: true,
});
