import {
  LayoutDashboard, Boxes, HardDrive, Wallet, BookOpen,
  UsersRound, Coins, BarChart3, Database, UserPlus,
} from 'lucide-react';

// 사용자 콘솔 사이드바 네비게이션.
// label 은 i18n 키(consoleUser.nav.*). path 는 /console 하위 상대경로.
export const userNavGroups = [
  {
    title: null,
    items: [
      { key: 'dashboard', icon: LayoutDashboard, path: 'dashboard' },
      { key: 'guide', icon: BookOpen, path: 'guide' },
    ],
  },
  {
    title: 'groupWorkload',
    items: [
      { key: 'sessions', icon: Boxes, path: 'sessions' },
    ],
  },
  {
    title: 'groupStorage',
    items: [
      { key: 'volumes', icon: HardDrive, path: 'volumes' },
      { key: 'datasets', icon: Database, path: 'datasets' },
      { key: 'wallet', icon: Wallet, path: 'wallet' },
    ],
  },
  {
    // 사이드바는 작업만 남긴다. 내 정보, 설정, 알림 센터는 개인 설정이라 탑바 아이콘으로 옮겼다(Topbar.jsx).
    title: 'groupAccount',
    items: [
      { key: 'joinGroup', icon: UserPlus, path: 'join-group' },
    ],
  },
];

// 그룹 관리(그룹 마스터 전용) 섹션이다. 활성 그룹 역할이 admin 일 때만 사이드바에 넣는다.
export const groupManageNavGroup = {
  title: 'groupManage',
  items: [
    { key: 'groupMembers', icon: UsersRound, path: 'group/members' },
    { key: 'groupBudget', icon: Coins, path: 'group/budget' },
    { key: 'groupUsage', icon: BarChart3, path: 'group/usage' },
  ],
};
