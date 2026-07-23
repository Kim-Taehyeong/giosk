// 컨테이너 이미지 카탈로그. 연결 방식(conns)은 이미지가 결정한다(사용자 선택 아님).
export const IMAGES = [
  {
    id: 'cuda124-vscode',
    name: 'CUDA 12.4 + VSCode',
    base: 'nvidia/cuda:12.4.0-devel',
    desc: 'VSCode 웹 IDE와 Jupyter, SSH를 모두 제공하는 범용 딥러닝 개발 환경입니다. 처음 시작하기에 가장 적합합니다.',
    conns: ['VSCode', 'Jupyter', 'SSH'],
    tags: ['CUDA 12.4', 'Python 3.10', 'GPU'],
    gpu: true,
  },
  {
    id: 'pytorch',
    name: 'PyTorch 2.3',
    base: 'pytorch/pytorch:2.3-cuda12.4',
    desc: 'PyTorch 2.3과 주요 라이브러리가 사전 설치된 학습 전용 이미지. Jupyter와 SSH로 접속합니다.',
    conns: ['Jupyter', 'SSH'],
    tags: ['PyTorch 2.3', 'CUDA 12.4', 'GPU'],
    gpu: true,
  },
  {
    id: 'vscode',
    name: 'VSCode Server',
    base: 'codercom/code-server',
    desc: '가벼운 VSCode 웹 IDE 환경. 코드 편집·디버깅 위주 작업에 적합하며 SSH도 지원합니다.',
    conns: ['VSCode', 'SSH'],
    tags: ['code-server', 'GPU'],
    gpu: true,
  },
  {
    id: 'ubuntu',
    name: 'Ubuntu 22.04 (CPU)',
    base: 'ubuntu:22.04',
    desc: '순수 Ubuntu 기본 환경. 데이터 다운로드·전처리 등 CPU 작업용이며 SSH로 접속합니다. 크레딧 무료.',
    conns: ['SSH'],
    tags: ['Ubuntu 22.04', 'CPU'],
    gpu: false,
  },
];

export const imageById = (id) => IMAGES.find((i) => i.id === id) || IMAGES[0];
