import { apiGet } from '../client';

// 물리 노드(SSH) 사양·로컬 캐시 데이터셋·노드별 home/dataset 사용량.
// 실 백엔드(라이브 K8s 노드 + DB 쿼터 오버레이). 캐시 데이터셋/실사용량은
// node-agent 보고 전까지 빈 값으로 내려온다(hybrid 전용).
export const getSshNodes = () => apiGet('/nodes/physical').then((d) => d.nodes || []);
