import { useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';

function Layout() {
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const baseLinkClass = 'px-3 py-2 rounded text-sm font-medium transition-colors';
  const activeClass = 'text-white bg-purple-600';
  const inactiveClass = 'text-gray-700 hover:text-gray-900 hover:bg-gray-100';

  const navLinks = (
    <>
      <NavLink
        to="/"
        end
        onClick={() => setMobileMenuOpen(false)}
        className={({ isActive }) =>
          `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
        }
      >
        时间线
      </NavLink>
      <NavLink
        to="/search"
        onClick={() => setMobileMenuOpen(false)}
        className={({ isActive }) =>
          `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
        }
      >
        搜索
      </NavLink>
    </>
  );

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="bg-white shadow-sm border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-14">
            <NavLink to="/" className="text-lg font-bold text-gray-900 tracking-tight">
              Phosche
            </NavLink>

            <div className="hidden md:flex items-center gap-1">
              {navLinks}
            </div>

            <button
              type="button"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="md:hidden inline-flex items-center justify-center p-2 rounded-md text-gray-500 hover:text-gray-700 hover:bg-gray-100 transition-colors cursor-pointer"
              aria-label="切换菜单"
            >
              {mobileMenuOpen ? (
                <svg
                  className="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              ) : (
                <svg
                  className="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M4 6h16M4 12h16M4 18h16"
                  />
                </svg>
              )}
            </button>
          </div>
        </div>

        {mobileMenuOpen && (
          <div className="md:hidden border-t border-gray-200 bg-white">
            <div className="max-w-6xl mx-auto px-4 py-2 flex flex-col gap-1">
              {navLinks}
            </div>
          </div>
        )}
      </nav>
      <main className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <Outlet />
      </main>
    </div>
  );
}

export default Layout;
