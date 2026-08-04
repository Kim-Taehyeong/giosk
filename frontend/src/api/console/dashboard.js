import { apiGet } from '../client';

// 사용자와 관리자 대시보드. 실 백엔드다(DB 집계에 Prometheus/DCGM 지표를 더한다).
// Prometheus 미연동 시 GPU 지표는 0/빈 추이로 안전하게 내려온다.
export const getUserDashboard = () => apiGet('/dashboard');
// 인프라 대시보드(클러스터 하드웨어)는 platform 전용이다. Topbar 배지와 함께 쓴다.
export const getAdminDashboard = () => apiGet('/admin/dashboard');
export const getInfraDashboard = () => apiGet('/admin/dashboard');
// 운영 대시보드(사용과 거버넌스)는 모든 관리 레벨이 보며 백엔드가 호출자 스코프로 필터한다.
export const getOpsDashboard = () => apiGet('/console/dashboard/ops');
