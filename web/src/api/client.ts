import axios from 'axios';

const BASE_URL = typeof window === 'undefined'
  ? 'http://localhost:8080/api'
  : '/api';

const apiClient = axios.create({
  baseURL: BASE_URL,
  timeout: 10000,
  adapter: 'fetch',
});

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    const msg = error.response?.data?.error?.message || error.message;
    return Promise.reject(new Error(msg));
  },
);

export default apiClient;
