import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchPhotoDetail } from '../api/photos';
import PhotoDetailModal from '../components/PhotoDetail';

function PhotoDetail() {
  const { '*': wildcard } = useParams<{ '*': string }>();
  const id = wildcard ? decodeURIComponent(wildcard).replace(/^\/+/, '') : '';
  const navigate = useNavigate();

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['photo', id],
    queryFn: () => fetchPhotoDetail(id!),
    enabled: !!id,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-32">
        <div className="animate-spin rounded-full h-10 w-10 border-[3px] border-purple-200 border-t-purple-600" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-4">
        <p className="text-gray-500 text-lg">加载失败</p>
        <button
          onClick={() => refetch()}
          className="px-5 py-2.5 bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors text-sm font-medium cursor-pointer"
        >
          重试
        </button>
      </div>
    );
  }

  return <PhotoDetailModal photo={data} onClose={() => navigate('/')} />;
}

export default PhotoDetail;
