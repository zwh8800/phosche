import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import ErrorBoundary from './components/ErrorBoundary';
import Timeline from './pages/Timeline';
import Search from './pages/Search';
import PhotoDetail from './pages/PhotoDetail';
import NotFound from './pages/NotFound';

const queryClient = new QueryClient();

function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<Timeline />} />
              <Route path="search" element={<Search />} />
              <Route path="photos/:id" element={<PhotoDetail />} />
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
