import { NavLink, Outlet } from 'react-router-dom';

function Layout() {
  const baseLinkClass = 'px-3 py-2 rounded text-sm font-medium transition-colors';
  const activeClass = 'text-white bg-purple-600';
  const inactiveClass = 'text-gray-700 hover:text-gray-900 hover:bg-gray-100';

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="bg-white shadow-sm border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-14">
            <NavLink to="/" className="text-lg font-bold text-gray-900 tracking-tight">
              Phosche
            </NavLink>
            <div className="flex items-center gap-1">
              <NavLink
                to="/"
                end
                className={({ isActive }) =>
                  `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
                }
              >
                时间线
              </NavLink>
              <NavLink
                to="/search"
                className={({ isActive }) =>
                  `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
                }
              >
                搜索
              </NavLink>
            </div>
          </div>
        </div>
      </nav>
      <main className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <Outlet />
      </main>
    </div>
  );
}

export default Layout;
