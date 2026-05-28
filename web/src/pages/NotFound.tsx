import { Link } from 'react-router-dom';

function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center">
      <p className="text-8xl font-bold text-purple-200">404</p>
      <h1 className="mt-4 text-2xl font-semibold text-gray-800">
        页面未找到
      </h1>
      <p className="mt-2 text-gray-500">
        您访问的页面不存在或已被移除
      </p>
      <Link
        to="/"
        className="mt-8 inline-flex items-center gap-2 rounded-lg bg-purple-600 px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-purple-700"
      >
        <svg
          className="h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M10 19l-7-7m0 0l7-7m-7 7h18"
          />
        </svg>
        返回首页
      </Link>
    </div>
  );
}

export default NotFound;
