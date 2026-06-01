import {
  defineConfig,
  minimal2023Preset as preset,
} from '@vite-pwa/assets-generator/config';

export default defineConfig({
  preset,
  images: 'public/favicon.svg',
  headLinkOptions: {
    preset: '2023',
  },
  overrideAssets: true,
  assetOptions: {
    padding: 0.1,
    background: '#ff2442',
  },
});
