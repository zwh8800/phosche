import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';

function Timeline() {
  return (
    <div>
      <h1>Timeline</h1>
    </div>
  );
}

function Search() {
  return (
    <div>
      <h1>Search</h1>
    </div>
  );
}

function PhotoDetail() {
  return (
    <div>
      <h1>Photo Detail</h1>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Timeline />} />
          <Route path="search" element={<Search />} />
          <Route path="photos/:id" element={<PhotoDetail />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
