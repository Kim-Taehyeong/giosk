import { apiPut } from '../client';

// 운영 중 조정 항목만 영속(유휴 타임아웃 + 단순 기능 토글). 무거운 항목은 설치시 고정이라 무시됨.
export const putSystemConfig = (body) => apiPut('/admin/config', body);
