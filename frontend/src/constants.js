export const OFFERINGS = [
  {
    id: 'h100',
    name: 'NVIDIA H100',
    vram: '80GB',
    flops: '66.91 TFlops',
    cpu: '8 Core',
    ram: '64 GB',
    price: 4379,
    series: 'H-Series',
    badge: 'Featured',
    available: true
  },
  {
    id: 'b200',
    name: 'NVIDIA B200',
    vram: '192GB',
    flops: '62.1 TFlops',
    cpu: '32 Core',
    ram: '242 GB',
    price: 13366,
    series: 'B-Series',
    badge: 'High Perf',
    available: true
  },
  {
    id: 'l40s',
    name: 'NVIDIA L40s',
    vram: '48GB',
    flops: '91.61 TFlops',
    cpu: '32 Core',
    ram: '64 GB',
    price: 2083,
    series: 'L-Series',
    badge: 'Value',
    available: true
  },
];

export const INITIAL_INSTANCES = [
  {
    id: 101,
    name: 'train-model-v1',
    offering: 'NVIDIA L40s',
    status: 'Running',
    ip: '192.168.0.14',
    tool: 'JupyterLab',
    uptime: '2h 15m',
    queue: null
  },
  {
    id: 102,
    name: 'data-prep-node',
    offering: 'NVIDIA T4',
    status: 'Pending',
    ip: '-',
    tool: 'SSH',
    uptime: '-',
    queue: 3
  },
  {
    id: 103,
    name: 'legacy-project',
    offering: 'NVIDIA L40s',
    status: 'Stopped',
    ip: '-',
    tool: 'VSCode Server',
    uptime: '-',
    queue: null
  },
];