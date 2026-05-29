import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import ErrorBoundary from './components/ErrorBoundary';
import Timeline from './pages/Timeline';
import Search from './pages/Search';
import PhotoDetail from './pages/PhotoDetail';
import NotFound from './pages/NotFound';

const queryClient = new QueryClient();

const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { index: true, element: <Timeline /> },
      { path: 'search', element: <Search /> },
      { path: 'photo/*', element: <PhotoDetail /> },
      { path: '*', element: <NotFound /> },
    ],
  },
]);

function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
